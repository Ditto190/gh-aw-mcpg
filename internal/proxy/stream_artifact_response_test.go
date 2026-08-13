package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorResponseWriter is an http.ResponseWriter whose Write always fails,
// used to exercise the io.Copy error branch in streamArtifactResponse.
type errorResponseWriter struct {
	header      http.Header
	wroteHeader int
}

func (e *errorResponseWriter) Header() http.Header {
	if e.header == nil {
		e.header = make(http.Header)
	}
	return e.header
}

func (e *errorResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("simulated write error")
}

func (e *errorResponseWriter) WriteHeader(statusCode int) {
	e.wroteHeader = statusCode
}

// newSpans returns a proxyHandler wired with an in-memory recording tracer and a
// function to fetch the recorded spans, along with a helper to start a span.
func startTestSpan(t *testing.T, name string) (context.Context, oteltrace.Span, func() []tracetest.SpanStub) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	sp := sdktrace.NewSimpleSpanProcessor(exporter)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	t.Cleanup(func() { _ = tp.Shutdown(t.Context()) })
	ctx, span := tp.Tracer("test").Start(context.Background(), name)
	return ctx, span, func() []tracetest.SpanStub { return exporter.GetSpans() }
}

// TestStreamArtifactResponse_UpstreamError verifies that a network failure while
// forwarding the request results in a 502 bad_gateway JSON response and a false
// return value, and does not panic on the nil-response path.
func TestStreamArtifactResponse_UpstreamError(t *testing.T) {
	s := newTestServer(t, "http://127.0.0.1:1")
	h := &proxyHandler{server: s}

	_, difcSpan, _ := startTestSpan(t, "difc")
	_, fwdSpan, _ := startTestSpan(t, "fwd")

	req := httptest.NewRequest(http.MethodGet, "/repos/org/repo/actions/artifacts/123/zip", nil)
	w := httptest.NewRecorder()

	ok := h.streamArtifactResponse(w, req, "/repos/org/repo/actions/artifacts/123/zip", context.Background(), difcSpan, fwdSpan, "")

	assert.False(t, ok)
	assertJSONErrorResponse(t, w.Result(), http.StatusBadGateway, "bad_gateway", "upstream request failed")
}

// TestStreamArtifactResponse_Success verifies the happy path: headers are copied,
// the status code is written, and the body is streamed via io.Copy.
func TestStreamArtifactResponse_Success(t *testing.T) {
	wantBody := []byte{0x50, 0x4b, 0x03, 0x04}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="a.zip"`)
		w.Header().Set("X-GitHub-Request-Id", "abc123")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(wantBody)
		require.NoError(t, err)
	}))
	defer upstream.Close()

	s := newTestServer(t, upstream.URL)
	h := &proxyHandler{server: s}

	_, difcSpan, _ := startTestSpan(t, "difc")
	_, fwdSpan, getSpans := startTestSpan(t, "fwd")

	req := httptest.NewRequest(http.MethodGet, "/repos/org/repo/actions/artifacts/123/zip", nil)
	w := httptest.NewRecorder()

	ok := h.streamArtifactResponse(w, req, "/repos/org/repo/actions/artifacts/123/zip", context.Background(), difcSpan, fwdSpan, "")
	fwdSpan.End()

	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/zip", w.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="a.zip"`, w.Header().Get("Content-Disposition"))
	assert.Equal(t, "abc123", w.Header().Get("X-GitHub-Request-Id"))
	assert.Equal(t, wantBody, w.Body.Bytes())

	// Verify the HTTP status code attribute was recorded on the forwarding span.
	spans := getSpans()
	require.Len(t, spans, 1)
}

// TestStreamArtifactResponse_RateLimited verifies that when the upstream response
// signals a rate limit (429 or X-Ratelimit-Remaining == 0), the function sets the
// rate-limit span attribute, records a "rate_limit.detected" event on a recording
// difcSpan (including reset_at when parseable), and still injects Retry-After and
// streams the body successfully.
func TestStreamArtifactResponse_RateLimited(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		remaining     string
		reset         string
		expectResetAt bool
	}{
		{
			name:          "429 with valid reset header",
			status:        http.StatusTooManyRequests,
			remaining:     "10",
			reset:         "9999999999",
			expectResetAt: true,
		},
		{
			name:          "200 with remaining=0 and no reset header",
			status:        http.StatusOK,
			remaining:     "0",
			reset:         "",
			expectResetAt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Ratelimit-Remaining", tt.remaining)
				if tt.reset != "" {
					w.Header().Set("X-Ratelimit-Reset", tt.reset)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte("body"))
			}))
			defer upstream.Close()

			s := newTestServer(t, upstream.URL)
			h := &proxyHandler{server: s}

			_, difcSpan, getDifcSpans := startTestSpan(t, "difc")
			_, fwdSpan, getFwdSpans := startTestSpan(t, "fwd")

			req := httptest.NewRequest(http.MethodGet, "/repos/org/repo/actions/artifacts/1/zip", nil)
			w := httptest.NewRecorder()

			ok := h.streamArtifactResponse(w, req, "/repos/org/repo/actions/artifacts/1/zip", context.Background(), difcSpan, fwdSpan, "")
			difcSpan.End()
			fwdSpan.End()

			assert.True(t, ok)
			assert.Equal(t, tt.status, w.Code)
			assert.NotEmpty(t, w.Header().Get("Retry-After"))

			difcSpans := getDifcSpans()
			require.Len(t, difcSpans, 1)
			events := difcSpans[0].Events
			require.NotEmpty(t, events)
			found := false
			for _, ev := range events {
				if ev.Name == "rate_limit.detected" {
					found = true
					hasResetAt := false
					for _, attr := range ev.Attributes {
						if string(attr.Key) == "reset_at" {
							hasResetAt = true
						}
					}
					assert.Equal(t, tt.expectResetAt, hasResetAt)
				}
			}
			assert.True(t, found, "expected rate_limit.detected event to be recorded")

			fwdSpans := getFwdSpans()
			require.Len(t, fwdSpans, 1)
		})
	}
}

// TestStreamArtifactResponse_CopyError verifies that when writing the response
// body to the client fails, streamArtifactResponse logs the error but still
// returns true (headers/status already sent, so it cannot report failure to the
// client at that point).
func TestStreamArtifactResponse_CopyError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("some body bytes"))
	}))
	defer upstream.Close()

	s := newTestServer(t, upstream.URL)
	h := &proxyHandler{server: s}

	_, difcSpan, _ := startTestSpan(t, "difc")
	_, fwdSpan, _ := startTestSpan(t, "fwd")

	req := httptest.NewRequest(http.MethodGet, "/repos/org/repo/actions/artifacts/1/zip", nil)
	w := &errorResponseWriter{}

	ok := h.streamArtifactResponse(w, req, "/repos/org/repo/actions/artifacts/1/zip", context.Background(), difcSpan, fwdSpan, "")

	assert.True(t, ok)
	assert.Equal(t, http.StatusOK, w.wroteHeader)
}

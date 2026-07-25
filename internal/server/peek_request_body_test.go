package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorOnReadReader is a test helper that returns an error on Read.
type errorOnReadReader struct {
	readErr error
}

func (r *errorOnReadReader) Read(_ []byte) (int, error) { return 0, r.readErr }
func (r *errorOnReadReader) Close() error               { return nil }

// errorOnCloseReader is a test helper that succeeds on Read but fails on Close.
type errorOnCloseReader struct {
	data     *bytes.Reader
	closeErr error
}

func (r *errorOnCloseReader) Read(p []byte) (int, error) { return r.data.Read(p) }
func (r *errorOnCloseReader) Close() error               { return r.closeErr }

// TestReadAndRestoreRequestBody verifies all branches of readAndRestoreRequestBody:
// nil/NoBody bodies, read errors, close errors, empty body, and non-empty body with
// body-restoration behaviour.
func TestReadAndRestoreRequestBody(t *testing.T) {
	t.Parallel()

	readErr := errors.New("simulated read error")
	closeErr := errors.New("simulated close error")

	tests := []struct {
		name        string
		buildReq    func() *http.Request
		wantBytes   []byte
		wantErr     error
		checkBody   bool // verify body is readable again after the call
		wantBodyVal string
	}{
		{
			name: "nil body returns nil",
			buildReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/", nil)
				req.Body = nil
				return req
			},
			wantBytes: nil,
			wantErr:   nil,
		},
		{
			name: "http.NoBody returns nil",
			buildReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/", nil)
				req.Body = http.NoBody
				return req
			},
			wantBytes: nil,
			wantErr:   nil,
		},
		{
			name: "non-empty body returns bytes and restores body",
			buildReq: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"method":"tools/list"}`))
			},
			wantBytes:   []byte(`{"method":"tools/list"}`),
			wantErr:     nil,
			checkBody:   true,
			wantBodyVal: `{"method":"tools/list"}`,
		},
		{
			name: "binary body restores body for re-reading",
			buildReq: func() *http.Request {
				content := []byte{0x00, 0x01, 0x02, 0xFF}
				return httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(content))
			},
			wantBytes: []byte{0x00, 0x01, 0x02, 0xFF},
			wantErr:   nil,
			checkBody: true,
		},
		{
			name: "empty body (reader at EOF) returns empty slice",
			buildReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(""))
				// httptest.NewRequest wraps an empty buffer in a ReadCloser rather than
				// using http.NoBody, so this exercises the len(b)==0 branch.
				return req
			},
			wantBytes: []byte{},
			wantErr:   nil,
		},
		{
			name: "read error propagates the error",
			buildReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/", nil)
				req.Body = &errorOnReadReader{readErr: readErr}
				return req
			},
			wantBytes: nil,
			wantErr:   readErr,
		},
		{
			name: "close error propagates the error",
			buildReq: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/", nil)
				req.Body = &errorOnCloseReader{
					data:     bytes.NewReader([]byte("some content")),
					closeErr: closeErr,
				}
				return req
			},
			wantBytes: nil,
			wantErr:   closeErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := tt.buildReq()

			got, err := readAndRestoreRequestBody(req)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantBytes, got)

			if tt.checkBody {
				// Verify that readAndRestoreRequestBody restored the body so it can be read again.
				require.NotNil(t, req.Body, "body should not be nil after read")
				assert.NotEqual(t, http.NoBody, req.Body, "body should be readable, not NoBody")

				restored, readErr := io.ReadAll(req.Body)
				require.NoError(t, readErr)

				if tt.wantBodyVal != "" {
					assert.Equal(t, tt.wantBodyVal, string(restored))
				} else {
					assert.Equal(t, tt.wantBytes, restored)
				}
			}
		})
	}
}

// TestReadAndRestoreRequestBody_BodyRestoredForMultipleReads confirms the fundamental
// contract: after readAndRestoreRequestBody returns, downstream handlers can still
// read the full body.
func TestReadAndRestoreRequestBody_BodyRestoredForMultipleReads(t *testing.T) {
	t.Parallel()

	body := `{"jsonrpc":"2.0","method":"tools/call","id":1}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))

	// First read
	b1, err := readAndRestoreRequestBody(req)
	require.NoError(t, err)
	assert.Equal(t, body, string(b1))

	// Body must still be fully readable after the read.
	b2, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(b2), "downstream handler should receive the complete body")
}

// TestReadAndRestoreRequestBody_EmptyBodySetsNoBody confirms that when the body is empty
// the request body is replaced with http.NoBody (not a lingering empty reader).
func TestReadAndRestoreRequestBody_EmptyBodySetsNoBody(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(""))

	got, err := readAndRestoreRequestBody(req)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.Equal(t, http.NoBody, req.Body, "empty body should be replaced with http.NoBody")
}

package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/stretchr/testify/assert"
)

// errorReadCloser is an io.ReadCloser whose Read always fails, used to exercise
// the io.ReadAll error path when reading a GraphQL request body in ServeHTTP.
type errorReadCloser struct{}

func (errorReadCloser) Read(_ []byte) (int, error) { return 0, errors.New("simulated read error") }
func (errorReadCloser) Close() error               { return nil }

// ─── ServeHTTP: GraphQL request body read failure → 400 ──────────────────────

// TestServeHTTP_GraphQLBodyReadError exercises the io.ReadAll error branch in
// ServeHTTP (handler.go lines 125-128) when the GraphQL POST request body
// cannot be read.
func TestServeHTTP_GraphQLBodyReadError(t *testing.T) {
	s := newTestServer(t, "http://unused")
	h := &proxyHandler{server: s}

	req := httptest.NewRequest(http.MethodPost, "/graphql", errorReadCloser{})
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "failed to read request body")
}

// ─── ServeHTTP: GraphQL introspection query, upstream unreachable ────────────

// TestServeHTTP_GraphQLIntrospectionUpstreamFailure exercises the "resp == nil"
// early-return branch in ServeHTTP's introspection passthrough (handler.go
// lines 145-147) when forwarding the introspection query to upstream fails.
func TestServeHTTP_GraphQLIntrospectionUpstreamFailure(t *testing.T) {
	s := newTestServer(t, "http://127.0.0.1:1") // connection refused
	h := &proxyHandler{server: s}

	gqlBody := []byte(`{"query":"{ __schema { types { name } } }"}`)
	req := httptest.NewRequest(http.MethodPost, "/graphql", nopReader(gqlBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

// nopReader wraps a byte slice as an io.Reader for use as a request body.
func nopReader(b []byte) io.Reader {
	return &fixedReader{data: b}
}

type fixedReader struct {
	data []byte
	pos  int
}

func (f *fixedReader) Read(p []byte) (int, error) {
	if f.pos >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}

// ─── Phase 4: LabelResponse error, enclave mode → writeEnclaveDenied ─────────

// TestHandleWithDIFC_LabelResponseError_EnclaveMode exercises the
// "s.enclave != nil" branch inside the Phase 4 error handler (handler.go
// lines 321-324), where an enclave-mode server responds with the generic
// enclave-denied error instead of falling back to a coarse result.
func TestHandleWithDIFC_LabelResponseError_EnclaveMode(t *testing.T) {
	upstream := mockUpstream(t, http.StatusOK, []interface{}{
		map[string]interface{}{"id": 1},
	})
	defer upstream.Close()

	g := &stubGuard{
		labelResourceResult: publicResource(),
		labelResourceOp:     difc.OperationRead,
		labelResponseErr:    errors.New("response labeling failed"),
	}
	s := newTestServerWithStub(t, upstream.URL, g, difc.EnforcementFilter)
	s.enclave = &enclaveState{}
	h := &proxyHandler{server: s}

	req := httptest.NewRequest(http.MethodGet, "/repos/org/repo/issues", nil)
	w := httptest.NewRecorder()
	h.handleWithDIFC(w, req, "/repos/org/repo/issues", "list_issues",
		map[string]interface{}{"owner": "org", "repo": "repo"}, nil)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "enclave_access_denied")
}

// ─── Phase 5: FilterAndConvertLabeledData error, non-enclave → empty response ─

// TestHandleWithDIFC_Phase5Error_NonEnclave exercises the Phase 5
// FilterAndConvertLabeledData error branch (handler.go lines 346-353) when
// the server is not in enclave mode: the response falls back to an empty
// result rather than the enclave-denied error.
func TestHandleWithDIFC_Phase5Error_NonEnclave(t *testing.T) {
	upstream := mockUpstream(t, http.StatusOK, []interface{}{
		map[string]interface{}{"id": 1},
	})
	defer upstream.Close()

	// A single (non-collection) LabeledData whose ToResult always fails
	// triggers the Phase 5 FilterAndConvertLabeledData error branch.
	g := &stubGuard{
		labelResourceResult: publicResource(),
		labelResourceOp:     difc.OperationRead,
		labelResponseData:   &errorToResultLabeledData{},
	}
	s := newTestServerWithStub(t, upstream.URL, g, difc.EnforcementFilter)
	h := &proxyHandler{server: s}

	req := httptest.NewRequest(http.MethodGet, "/repos/org/repo/issues", nil)
	w := httptest.NewRecorder()
	h.handleWithDIFC(w, req, "/repos/org/repo/issues", "list_issues",
		map[string]interface{}{"owner": "org", "repo": "repo"}, nil)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", w.Body.String())
}

// ─── Phase 5: FilterAndConvertLabeledData error, enclave mode → denied ────────

// TestHandleWithDIFC_Phase5Error_EnclaveMode exercises the "s.enclave != nil"
// branch inside the Phase 5 error handler (handler.go lines 348-351).
func TestHandleWithDIFC_Phase5Error_EnclaveMode(t *testing.T) {
	upstream := mockUpstream(t, http.StatusOK, []interface{}{
		map[string]interface{}{"id": 1},
	})
	defer upstream.Close()

	g := &stubGuard{
		labelResourceResult: publicResource(),
		labelResourceOp:     difc.OperationRead,
		labelResponseData:   &errorToResultLabeledData{},
	}
	s := newTestServerWithStub(t, upstream.URL, g, difc.EnforcementFilter)
	s.enclave = &enclaveState{}
	h := &proxyHandler{server: s}

	req := httptest.NewRequest(http.MethodGet, "/repos/org/repo/issues", nil)
	w := httptest.NewRecorder()
	h.handleWithDIFC(w, req, "/repos/org/repo/issues", "list_issues",
		map[string]interface{}{"owner": "org", "repo": "repo"}, nil)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "enclave_access_denied")
}

// errorToResultLabeledData is a LabeledData implementation whose ToResult
// always fails. It is used to deterministically exercise the Phase 5
// FilterAndConvertLabeledData error branch in handleWithDIFC.
type errorToResultLabeledData struct{}

func (errorToResultLabeledData) Overall() *difc.LabeledResource {
	return difc.NewLabeledResource("error-labeled-data")
}

func (errorToResultLabeledData) ToResult() (interface{}, error) {
	return nil, errors.New("simulated ToResult failure")
}

// ─── Phase 6: json.Marshal failure on the final filtered response ────────────

// TestHandleWithDIFC_FinalMarshalError exercises the json.Marshal error branch
// at the very end of handleWithDIFC (handler.go lines 421-424), where the
// filtered finalData cannot be serialized back to JSON.
func TestHandleWithDIFC_FinalMarshalError(t *testing.T) {
	upstream := mockUpstream(t, http.StatusOK, map[string]interface{}{
		"name": "README.md",
	})
	defer upstream.Close()

	// SimpleLabeledData.ToResult returns the Data field verbatim; make it a
	// value that fails to marshal to JSON (e.g. a channel) so that the final
	// json.Marshal(finalData) call in handleWithDIFC fails.
	simple := &difc.SimpleLabeledData{
		Data:   make(chan int),
		Labels: publicResource(),
	}
	g := &stubGuard{
		labelResourceResult: publicResource(),
		labelResourceOp:     difc.OperationRead,
		labelResponseData:   simple,
	}
	s := newTestServerWithStub(t, upstream.URL, g, difc.EnforcementFilter)
	h := &proxyHandler{server: s}

	req := httptest.NewRequest(http.MethodGet, "/repos/org/repo/contents/README.md", nil)
	w := httptest.NewRecorder()
	h.handleWithDIFC(w, req, "/repos/org/repo/contents/README.md", "get_file_contents",
		map[string]interface{}{"owner": "org", "repo": "repo"}, nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to serialize filtered response")
}

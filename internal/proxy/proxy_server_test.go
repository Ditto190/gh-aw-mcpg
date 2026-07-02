package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNew_EmptyWasmPath verifies that New returns an error immediately when
// WasmPath is empty, before attempting any WASM or guard initialization.
func TestNew_EmptyWasmPath(t *testing.T) {
	_, err := New(context.Background(), Config{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "guard WASM path is required")
}

// TestNew_InvalidWasmPath verifies that New returns an error when the WASM
// file does not exist, covering the guard-load error path and the default
// GitHubAPIURL fallback branch.
func TestNew_InvalidWasmPath(t *testing.T) {
	_, err := New(context.Background(), Config{
		WasmPath: "/nonexistent/path/guard.wasm",
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to load WASM guard")
}

// TestNew_InvalidDIFCMode verifies that New logs a warning and continues when
// an unrecognised DIFCMode is supplied, then fails at the WASM-load step.
// This covers the difcParseErr != nil branch inside New.
func TestNew_InvalidDIFCMode(t *testing.T) {
	_, err := New(context.Background(), Config{
		WasmPath: "/nonexistent/path/guard.wasm",
		DIFCMode: "not-a-valid-mode",
	})
	require.Error(t, err)
	// Function continues past the DIFC warning and fails at WASM load.
	assert.ErrorContains(t, err, "failed to load WASM guard")
}

// TestHandler_ServesRequests verifies that Handler returns a non-nil
// http.Handler that correctly handles requests. The health-check path is used
// because it is handled entirely within the proxyHandler without requiring an
// upstream GitHub connection or WASM guard evaluation.
func TestHandler_ServesRequests(t *testing.T) {
	s := newTestServer(t, "http://unused")
	h := s.Handler()
	require.NotNil(t, h, "Handler should return a non-nil http.Handler")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}

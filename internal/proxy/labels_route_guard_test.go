package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/guard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findGuardWasm locates the compiled GitHub guard WASM module, or skips the
// test when it has not been built (`make build` in guards/github-guard).
func findGuardWasm(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("AWMG_WASM_GUARD_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		t.Fatalf("AWMG_WASM_GUARD_PATH=%q does not exist", p)
	}
	for _, loc := range []string{
		"../../guards/github-guard/rust-guard/target/wasm32-wasip1/release/github_guard.wasm",
		"../../guards/github-guard/github-guard-rust.wasm",
		"/guards/github/00-github-guard.wasm",
	} {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}
	t.Skip("WASM guard not built (run `make build` in guards/github-guard)")
	return ""
}

// labelsUpstream serves the minimal GitHub API surface the guard needs to
// label a repository label listing: repository visibility lookups plus the
// labels collection itself.
func labelsUpstream(t *testing.T, labels []map[string]interface{}) *httptest.Server {
	t.Helper()
	publicRepo := map[string]interface{}{
		"full_name":  "testorg/testrepo",
		"private":    false,
		"visibility": "public",
		"owner":      map[string]interface{}{"login": "testorg"},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		switch {
		case r.URL.Path == "/repos/testorg/testrepo/labels":
			enc.Encode(labels) //nolint:errcheck
		case r.URL.Path == "/search/repositories":
			enc.Encode(map[string]interface{}{ //nolint:errcheck
				"total_count": 1,
				"items":       []interface{}{publicRepo},
			})
		case r.URL.Path == "/repos/testorg/testrepo":
			enc.Encode(publicRepo) //nolint:errcheck
		default:
			enc.Encode(map[string]interface{}{}) //nolint:errcheck
		}
	}))
}

// TestHandler_ListLabels_RealGuard_ApprovedIntegrity is an end-to-end
// regression test for the REST labels route. With the real WASM guard and an
// `{repos: public, min-integrity: approved}` policy in filter mode, the proxy
// must return every upstream label. Previously the route used the plural tool
// name `list_labels`, which no guard rule recognises, so the response received
// only baseline `none` integrity and filter mode replaced it with `[]`.
func TestHandler_ListLabels_RealGuard_ApprovedIntegrity(t *testing.T) {
	wasmPath := findGuardWasm(t)

	labels := []map[string]interface{}{
		{"id": 1, "name": "bug", "color": "d73a4a", "description": "Something isn't working"},
		{"id": 2, "name": "enhancement", "color": "a2eeef", "description": "New feature or request"},
		{"id": 3, "name": "question", "color": "d876e3", "description": "Further information is requested"},
	}
	upstream := labelsUpstream(t, labels)
	defer upstream.Close()

	s, err := New(context.Background(), Config{
		WasmPath:     wasmPath,
		Policy:       `{"allow-only":{"repos":"public","min-integrity":"approved"}}`,
		GitHubToken:  "test-token",
		GitHubAPIURL: upstream.URL,
		DIFCMode:     "filter",
	})
	require.NoError(t, err)
	require.NotNil(t, s)
	t.Cleanup(func() {
		if wg, ok := s.guard.(*guard.WasmGuard); ok {
			assert.NoError(t, wg.Close(context.Background()))
		}
	})

	// Sanity: the route under test must resolve to the guard's tool name.
	m := MatchRoute("/repos/testorg/testrepo/labels")
	require.NotNil(t, m)
	require.Equal(t, "list_label", m.ToolName)

	req := httptest.NewRequest(http.MethodGet, "/repos/testorg/testrepo/labels", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, len(labels), "all upstream labels must survive filter mode")

	names := make([]string, 0, len(got))
	for _, l := range got {
		names = append(names, l["name"].(string))
	}
	assert.ElementsMatch(t, []string{"bug", "enhancement", "question"}, names)
}

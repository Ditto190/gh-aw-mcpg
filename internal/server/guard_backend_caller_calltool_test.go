package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/launcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newGuardCallToolBackend creates a mock HTTP MCP backend for guardBackendCaller.CallTool
// tests. It always handles "initialize" and "notifications/initialized" automatically,
// delegating all other methods (e.g. "tools/call") to handleMethod.
func newGuardCallToolBackend(t *testing.T, serverName string, handleMethod func(w http.ResponseWriter, method string, reqID interface{}, params interface{})) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			return
		}
		if len(body) == 0 {
			http.Error(w, "empty request body", http.StatusBadRequest)
			return
		}

		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
			return
		}
		method, _ := req["method"].(string)
		if method == "initialize" {
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]interface{}{},
					"serverInfo":      map[string]interface{}{"name": serverName, "version": "1.0"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			// The SDK stores the negotiated session from initialize responses and
			// reuses it on later requests, so tests need to provide one here.
			w.Header().Set("Mcp-Session-Id", "guard-calltool-test-session")
			require.NoError(t, json.NewEncoder(w).Encode(resp))
			return
		}
		if method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if method == "server/discover" {
			// The SDK probes for this method (go-sdk >= v1.7.0); respond with a
			// JSON-RPC "method not found" error so the client falls back to the
			// legacy initialize flow.
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"error": map[string]interface{}{
					"code":    -32601,
					"message": `method not found: "server/discover"`,
				},
			}))
			return
		}
		handleMethod(w, method, req["id"], req["params"])
	}))
}

// newGuardCallToolLauncher creates a launcher wired to a single HTTP backend server.
func newGuardCallToolLauncher(t *testing.T, serverID, backendURL string) *launcher.Launcher {
	t.Helper()
	cfg := &config.Config{
		Servers: map[string]*config.ServerConfig{
			serverID: {Type: "http", URL: backendURL},
		},
	}
	l := launcher.New(context.Background(), cfg)
	t.Cleanup(func() { l.Close() })
	return l
}

// TestGuardBackendCaller_CallTool_MetadataSuccess verifies that CallTool on a
// non-synthetic tool name is forwarded to the backend as a tools/call request and
// the raw result is returned.
func TestGuardBackendCaller_CallTool_MetadataSuccess(t *testing.T) {
	backend := newGuardCallToolBackend(t, "metadata-server", func(w http.ResponseWriter, method string, reqID interface{}, params interface{}) {
		assert.Equal(t, "tools/call", method)

		paramsMap, ok := params.(map[string]interface{})
		require.True(t, ok, "expected params to be a map")
		assert.Equal(t, "get_issue", paramsMap["name"])

		args, ok := paramsMap["arguments"].(map[string]interface{})
		require.True(t, ok, "expected arguments to be a map")
		assert.Equal(t, "42", args["issue_number"])

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      reqID,
			"result": map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "issue body"},
				},
			},
		}))
	})
	defer backend.Close()

	l := newGuardCallToolLauncher(t, "metadata-server", backend.URL)
	us := &UnifiedServer{launcher: l}
	caller := &guardBackendCaller{server: us, serverID: "metadata-server", ctx: context.Background()}

	result, err := caller.CallTool(context.Background(), "get_issue", map[string]interface{}{
		"issue_number": "42",
	})
	require.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok, "expected result to be a map, got %T", result)
	content, ok := resultMap["content"].([]interface{})
	require.True(t, ok, "expected content field to be a slice")
	require.Len(t, content, 1)
}

// TestGuardBackendCaller_CallTool_UsesSessionIDFromContext verifies that CallTool
// reads the session ID from g.ctx (not the ctx parameter) when routing the request,
// matching the documented behavior of executeBackendToolCall(g.ctx, ...).
func TestGuardBackendCaller_CallTool_UsesSessionIDFromContext(t *testing.T) {
	backend := newGuardCallToolBackend(t, "session-server", func(w http.ResponseWriter, method string, reqID interface{}, _ interface{}) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      reqID,
			"result": map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "ok"},
				},
			},
		}))
	})
	defer backend.Close()

	l := newGuardCallToolLauncher(t, "session-server", backend.URL)
	us := &UnifiedServer{launcher: l}

	// Put a distinct session ID on the guard's stored ctx (g.ctx), separate from the
	// ctx argument passed to CallTool, to verify the implementation prefers g.ctx.
	guardCtx := context.WithValue(context.Background(), SessionIDContextKey, "guard-session-1")
	caller := &guardBackendCaller{server: us, serverID: "session-server", ctx: guardCtx}

	// Passing a background context here (with no session value) to CallTool's ctx
	// argument; the call should still succeed because the implementation uses g.ctx
	// for session resolution, not the passed-in ctx.
	result, err := caller.CallTool(context.Background(), "list_files", nil)
	require.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok, "expected map[string]interface{}, got %T: %#v", result, result)
	content, ok := resultMap["content"].([]interface{})
	require.True(t, ok, "expected content field to be a slice")
	require.Len(t, content, 1)
}

// TestGuardBackendCaller_CallTool_BackendError verifies that a JSON-RPC error
// returned by the backend is surfaced as a Go error from CallTool.
func TestGuardBackendCaller_CallTool_BackendError(t *testing.T) {
	backend := newGuardCallToolBackend(t, "error-server", func(w http.ResponseWriter, method string, reqID interface{}, _ interface{}) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      reqID,
			"error": map[string]interface{}{
				"code":    -32602,
				"message": "Invalid params",
			},
		}))
	})
	defer backend.Close()

	l := newGuardCallToolLauncher(t, "error-server", backend.URL)
	us := &UnifiedServer{launcher: l}
	caller := &guardBackendCaller{server: us, serverID: "error-server", ctx: context.Background()}

	_, err := caller.CallTool(context.Background(), "broken_tool", map[string]interface{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid params")
}

// TestGuardBackendCaller_CallTool_LauncherError verifies that CallTool surfaces an
// error when the backend server is not registered in the launcher.
func TestGuardBackendCaller_CallTool_LauncherError(t *testing.T) {
	cfg := &config.Config{Servers: map[string]*config.ServerConfig{}}
	l := launcher.New(context.Background(), cfg)
	defer l.Close()

	us := &UnifiedServer{launcher: l}
	caller := &guardBackendCaller{server: us, serverID: "missing-server", ctx: context.Background()}

	_, err := caller.CallTool(context.Background(), "some_tool", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to backend missing-server")
}

// TestGuardBackendCaller_CallTool_CollaboratorPermissionInterception verifies that
// CallTool intercepts the synthetic "get_collaborator_permission" tool name and
// routes it to callCollaboratorPermission instead of the backend, even when a
// (non-functional) launcher/server is present. This complements the more detailed
// collaborator-permission tests in collaborator_permission_test.go by confirming the
// dispatch/interception branch specifically.
func TestGuardBackendCaller_CallTool_CollaboratorPermissionInterception(t *testing.T) {
	// No backend registered for this serverID; if CallTool routed this to the
	// backend (instead of intercepting it), it would fail with a launcher connect
	// error rather than an args-validation error.
	cfg := &config.Config{Servers: map[string]*config.ServerConfig{}}
	l := launcher.New(context.Background(), cfg)
	defer l.Close()

	us := &UnifiedServer{launcher: l}
	caller := &guardBackendCaller{server: us, serverID: "irrelevant-server", ctx: context.Background()}

	_, err := caller.CallTool(context.Background(), "get_collaborator_permission", "not-a-map")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected args type")
}

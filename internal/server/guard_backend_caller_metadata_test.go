package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGuardBackendCallerCallTool_MetadataPath covers the metadata (non
// get_collaborator_permission) branch of guardBackendCaller.CallTool, which
// delegates to executeBackendToolCall via the server's launcher. Before this
// test, this branch (unified.go lines 281-285) had zero coverage.
func TestGuardBackendCallerCallTool_MetadataPath(t *testing.T) {
	t.Run("successful metadata call returns backend result", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				// Some protocol traffic (e.g. notifications) may have no body;
				// respond with 202 Accepted and skip further processing.
				w.WriteHeader(http.StatusAccepted)
				return
			}

			method, _ := req["method"].(string)
			w.Header().Set("Content-Type", "application/json")

			switch method {
			case "initialize":
				json.NewEncoder(w).Encode(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      req["id"],
					"result": map[string]interface{}{
						"protocolVersion": "2024-11-05",
						"capabilities":    map[string]interface{}{},
						"serverInfo": map[string]interface{}{
							"name":    "test-backend",
							"version": "1.0.0",
						},
					},
				})
			case "tools/call":
				params, _ := req["params"].(map[string]interface{})
				assert.Equal("get_issue", params["name"])
				json.NewEncoder(w).Encode(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      req["id"],
					"result": map[string]interface{}{
						"content": []map[string]interface{}{
							{"type": "text", "text": "issue metadata"},
						},
						"isError": false,
					},
				})
			case "notifications/initialized":
				w.WriteHeader(http.StatusAccepted)
			default:
				w.WriteHeader(http.StatusAccepted)
			}
		}))
		defer backend.Close()

		cfg := &config.Config{
			Servers: map[string]*config.ServerConfig{
				"github": {
					Type: "http",
					URL:  backend.URL,
				},
			},
		}

		us, err := NewUnified(context.Background(), cfg)
		require.NoError(err)
		require.NotNil(us)

		ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
		caller := &guardBackendCaller{
			server:   us,
			serverID: "github",
			ctx:      ctx,
		}

		result, err := caller.CallTool(context.Background(), "get_issue", map[string]interface{}{
			"owner": "myorg",
			"repo":  "myrepo",
			"issue": 1,
		})

		require.NoError(err)
		require.NotNil(result)

		resultMap, ok := result.(map[string]interface{})
		require.True(ok, "expected result to be map[string]interface{}, got %T", result)
		assert.Equal(false, resultMap["isError"])
	})

	t.Run("backend error is propagated", func(t *testing.T) {
		require := require.New(t)

		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusAccepted)
				return
			}

			method, _ := req["method"].(string)
			w.Header().Set("Content-Type", "application/json")

			switch method {
			case "initialize":
				json.NewEncoder(w).Encode(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      req["id"],
					"result": map[string]interface{}{
						"protocolVersion": "2024-11-05",
						"capabilities":    map[string]interface{}{},
						"serverInfo": map[string]interface{}{
							"name":    "test-backend",
							"version": "1.0.0",
						},
					},
				})
			case "tools/call":
				json.NewEncoder(w).Encode(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      req["id"],
					"error": map[string]interface{}{
						"code":    -32000,
						"message": "backend tool failure",
					},
				})
			default:
				w.WriteHeader(http.StatusAccepted)
			}
		}))
		defer backend.Close()

		cfg := &config.Config{
			Servers: map[string]*config.ServerConfig{
				"github": {
					Type: "http",
					URL:  backend.URL,
				},
			},
		}

		us, err := NewUnified(context.Background(), cfg)
		require.NoError(err)
		require.NotNil(us)

		ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
		caller := &guardBackendCaller{
			server:   us,
			serverID: "github",
			ctx:      ctx,
		}

		result, err := caller.CallTool(context.Background(), "get_issue", map[string]interface{}{
			"owner": "myorg",
			"repo":  "myrepo",
			"issue": 1,
		})

		require.Error(err)
		require.Nil(result)
		require.Contains(err.Error(), "backend tool failure")
	})

	t.Run("connection failure to unknown server is propagated", func(t *testing.T) {
		require := require.New(t)

		cfg := &config.Config{
			Servers: map[string]*config.ServerConfig{},
		}

		us, err := NewUnified(context.Background(), cfg)
		require.NoError(err)
		require.NotNil(us)

		ctx := context.WithValue(context.Background(), SessionIDContextKey, "test-session")
		caller := &guardBackendCaller{
			server:   us,
			serverID: "nonexistent-server",
			ctx:      ctx,
		}

		result, err := caller.CallTool(context.Background(), "some_tool", map[string]interface{}{})

		require.Error(err)
		require.Nil(result)
	})
}

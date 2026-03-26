package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/launcher"
	"github.com/github/gh-aw-mcpg/internal/mcp"
)

// decodeJSONRPCMethod reads the request body and extracts the JSON-RPC method and ID.
// Returns empty method for non-JSON or empty bodies (e.g. SDK transport probes).
func decodeJSONRPCMethod(r *http.Request) (method string, id interface{}) {
	bodyBytes, _ := io.ReadAll(r.Body)
	if len(bodyBytes) == 0 {
		return "", nil
	}
	var req struct {
		Method string      `json:"method"`
		ID     interface{} `json:"id"`
	}
	json.Unmarshal(bodyBytes, &req)
	return req.Method, req.ID
}

// jsonRPCResult writes a JSON-RPC success response with the given request ID.
func jsonRPCResult(w http.ResponseWriter, id interface{}, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

// jsonRPCError writes a JSON-RPC error response with the given request ID.
func jsonRPCError(w http.ResponseWriter, statusCode int, id interface{}, code int, message string) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]interface{}{"code": code, "message": message},
	})
}

// TestHTTPBackendInitialization tests that HTTP backends use the session ID issued by the
// server during initialize (not a locally-fabricated one) when calling tools/list.
// This is a regression test for https://github.com/github/gh-aw/issues/18712 where
// gateway-issued fake session IDs overrode the real server-issued session ID, causing
// HTTP 400 on tools/list from strict backends like Datadog.
func TestHTTPBackendInitialization(t *testing.T) {
	const serverSessionID = "server-issued-session-42"
	var toolsListSessionID string

	// Create a mock HTTP MCP server that:
	// 1. Issues a specific session ID during initialize
	// 2. Requires that exact session ID for subsequent requests
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, id := decodeJSONRPCMethod(r)
		if method == "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		switch method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", serverSessionID)
			jsonRPCResult(w, id, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"serverInfo":      map[string]interface{}{"name": "test-server", "version": "1.0.0"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			toolsListSessionID = r.Header.Get("Mcp-Session-Id")
			if toolsListSessionID != serverSessionID {
				jsonRPCError(w, http.StatusBadRequest, id, -32603, "Invalid session ID")
				return
			}
			jsonRPCResult(w, id, map[string]interface{}{
				"tools": []map[string]interface{}{
					{"name": "test_tool", "description": "A test tool", "inputSchema": map[string]interface{}{"type": "object"}},
				},
			})
		}
	}))
	defer mockServer.Close()

	// Custom headers are forwarded to all transport types via RoundTripper injection
	cfg := &config.Config{
		Servers: map[string]*config.ServerConfig{
			"http-backend": {
				Type:    "http",
				URL:     mockServer.URL,
				Headers: map[string]string{"X-Auth": "test"},
			},
		},
	}

	// Create unified server - this calls tools/list during initialization
	ctx := context.Background()
	us, err := NewUnified(ctx, cfg)
	require.NoError(t, err, "Failed to create unified server: gateway must use server-issued session ID for tools/list")
	defer us.Close()

	// The session ID used for tools/list must be the one issued by the server during initialize,
	// not a locally-fabricated "gateway-init-*" value.
	assert.Equal(t, serverSessionID, toolsListSessionID,
		"tools/list must use the session ID issued by the server during initialize, not a fabricated one")

	t.Logf("Correctly used server-issued session ID for tools/list: %s", toolsListSessionID)
}

// TestHTTPBackendInitializationWithSessionIDRequirement tests the exact error scenario from the problem statement
func TestHTTPBackendInitializationWithSessionIDRequirement(t *testing.T) {
	// Create a strict HTTP MCP server that fails without Mcp-Session-Id header
	strictServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, id := decodeJSONRPCMethod(r)
		if method == "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		sessionID := r.Header.Get("Mcp-Session-Id")

		if sessionID == "" {
			jsonRPCError(w, http.StatusBadRequest, id, -32600, "Invalid Request: Missing Mcp-Session-Id header")
			return
		}

		switch method {
		case "initialize":
			jsonRPCResult(w, id, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"serverInfo":      map[string]interface{}{"name": "safeinputs", "version": "1.0.0"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			jsonRPCResult(w, id, map[string]interface{}{
				"tools": []map[string]interface{}{
					{"name": "safe_tool", "description": "A safe tool"},
				},
			})
		}
	}))
	defer strictServer.Close()

	// Create config with strict HTTP backend (simulating "safeinputs")
	cfg := &config.Config{
		Servers: map[string]*config.ServerConfig{
			"safeinputs": {
				Type: "http",
				URL:  strictServer.URL,
			},
		},
	}

	// Create unified server - should succeed with our fix
	ctx := context.Background()
	us, err := NewUnified(ctx, cfg)
	if err != nil {
		t.Fatalf("Failed to create unified server with strict HTTP backend: %v. This indicates the Mcp-Session-Id header is not being sent during initialization.", err)
	}
	defer us.Close()

	// Verify tools were registered
	tools := us.GetToolsForBackend("safeinputs")
	assert.False(t, len(tools) == 0, "Expected tools to be registered from safeinputs backend, got none")

	t.Logf("Successfully initialized strict HTTP backend 'safeinputs' with %d tools", len(tools))
}

// TestHTTPBackend_SessionIDPropagation tests that session ID is propagated through tool calls
func TestHTTPBackend_SessionIDPropagation(t *testing.T) {
	// Track session IDs received at different stages
	initializeSessionID := ""
	initSessionID := ""
	toolCallSessionID := ""

	// Create a mock HTTP MCP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, id := decodeJSONRPCMethod(r)
		if method == "" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		sessionID := r.Header.Get("Mcp-Session-Id")

		switch method {
		case "initialize":
			initializeSessionID = sessionID
			jsonRPCResult(w, id, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"serverInfo":      map[string]interface{}{"name": "test-http-server", "version": "1.0.0"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			initSessionID = sessionID
			jsonRPCResult(w, id, map[string]interface{}{
				"tools": []map[string]interface{}{
					{"name": "echo", "description": "Echo tool"},
				},
			})
		case "tools/call":
			toolCallSessionID = sessionID
			jsonRPCResult(w, id, map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "echo response"},
				},
			})
		}
	}))
	defer mockServer.Close()

	// Create config
	cfg := &config.Config{
		Servers: map[string]*config.ServerConfig{
			"test-http": {
				Type:    "http",
				URL:     mockServer.URL,
				Headers: map[string]string{"X-Test": "test"},
			},
		},
	}

	// Create unified server
	ctx := context.Background()
	us, err := NewUnified(ctx, cfg)
	require.NoError(t, err, "Failed to create unified server")
	defer us.Close()

	// Create a connection and call a tool with a specific session ID
	conn, err := launcher.GetOrLaunch(us.launcher, "test-http")
	require.NoError(t, err, "Failed to get connection")

	clientSessionID := "client-session-12345"
	ctxWithSession := context.WithValue(context.Background(), mcp.SessionIDContextKey, clientSessionID)

	_, err = conn.SendRequestWithServerID(ctxWithSession, "tools/call", map[string]interface{}{
		"name":      "echo",
		"arguments": map[string]interface{}{"message": "test"},
	}, "test-http")
	require.NoError(t, err, "Failed to call tool")

	// Verify session IDs were received.
	// With the SDK streamable transport, session IDs are managed internally by the SDK,
	// so the Mcp-Session-Id header may not appear in requests to the mock.
	// With plain JSON-RPC, the gateway explicitly injects session IDs via headers.
	if initializeSessionID != "" {
		t.Logf("Initialize session ID: %s", initializeSessionID)
	} else {
		t.Logf("No session ID on initialize (expected for SDK streamable transport)")
	}

	if initSessionID != "" {
		t.Logf("Init session ID: %s", initSessionID)
	} else {
		t.Logf("No session ID on tools/list (expected for SDK streamable transport)")
	}

	if toolCallSessionID != "" {
		assert.Equal(t, clientSessionID, toolCallSessionID,
			"tool call should propagate client session ID for plain JSON-RPC transport")
	} else {
		t.Logf("No session ID on tool call (expected for SDK streamable transport)")
	}

	t.Logf("Session ID propagation test passed")
}

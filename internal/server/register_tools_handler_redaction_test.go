package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/mcp"
	"github.com/github/gh-aw-mcpg/internal/sanitize"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolCallLogLines returns only the log lines emitted by the
// registerToolsFromBackendContext handler closure itself (the "MCP tool
// call ..." lines), filtering out unrelated lines logged by other
// middleware (e.g. the payload jq middleware also logs the raw backend
// error at debug level for its own bookkeeping). This keeps assertions
// about redaction focused on the behavior under test. Callers must pair
// this with a require.NotEmpty check on the result so a wording change to
// the filtered log messages fails the test loudly instead of silently
// matching zero lines.
func toolCallLogLines(logOutput string) string {
	var kept []string
	for _, line := range strings.Split(logOutput, "\n") {
		if strings.Contains(line, "MCP tool call") || strings.Contains(line, "Failed to unmarshal tool arguments") {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// TestRegisterToolsFromBackend_HandlerInvocation_Redaction exercises the
// argument-parse-error, request-log, and response-log branches of the tool
// handler closure created inside registerToolsFromBackendContext
// (internal/server/tool_registry.go), under both the normal (non-redacted)
// and enclave-session (redacted) code paths. These branches are otherwise
// only reached indirectly via the MCP transport in integration tests and
// were previously uncovered by unit tests.
func TestRegisterToolsFromBackend_HandlerInvocation_Redaction(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		method, _ := req["method"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]interface{}{},
					"serverInfo":      map[string]interface{}{"name": "redaction-backend", "version": "1.0"},
				},
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]interface{}{
					"tools": []map[string]interface{}{
						{
							"name":        "echo",
							"description": "echoes",
							"inputSchema": map[string]interface{}{
								"type":       "object",
								"properties": map[string]interface{}{"message": map[string]interface{}{"type": "string"}},
							},
						},
						{
							"name":        "boom",
							"description": "always errors",
							"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
						},
					},
				},
			})
		case "tools/call":
			params, _ := req["params"].(map[string]interface{})
			name, _ := params["name"].(string)
			if name == "boom" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"jsonrpc": "2.0", "id": req["id"],
					"error": map[string]interface{}{"code": -32000, "message": "simulated backend failure"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": "echoed-response-sentinel-value"}},
				},
			})
		}
	}))
	defer backend.Close()

	cfg := &config.Config{
		Servers: map[string]*config.ServerConfig{
			"redaction-backend": {
				Type: "http",
				URL:  backend.URL,
			},
		},
	}

	us, err := NewUnified(context.Background(), cfg)
	require.NoError(t, err)
	defer us.Close()

	require.NoError(t, us.registerToolsFromBackend("redaction-backend"))

	us.toolsMu.RLock()
	echoTool := us.tools["redaction-backend___echo"]
	boomTool := us.tools["redaction-backend___boom"]
	us.toolsMu.RUnlock()
	require.NotNil(t, echoTool)
	require.NotNil(t, echoTool.Handler)
	require.NotNil(t, boomTool)
	require.NotNil(t, boomTool.Handler)

	t.Run("malformed arguments under enclave session take the redacted parse-error branch", func(t *testing.T) {
		assert := assert.New(t)
		require := require.New(t)

		ctx := mcp.WithEnclaveSession(context.WithValue(context.Background(), SessionIDContextKey, "redact-parse-session"))
		req := &sdk.CallToolRequest{
			Params: &sdk.CallToolParamsRaw{
				Name:      "redaction-backend___echo",
				Arguments: json.RawMessage(`{not valid json`),
			},
		}
		var result *sdk.CallToolResult
		var data interface{}
		var err error
		logOutput := captureServerLog(t, func() {
			result, data, err = echoTool.Handler(ctx, req, nil)
		})
		handlerLog := toolCallLogLines(logOutput)
		require.NotEmpty(handlerLog, "expected at least one tool-call log line; log message text may have drifted from toolCallLogLines' filter")
		require.Error(err)
		require.NotNil(result)
		assert.True(result.IsError)
		assert.Nil(data)
		assert.NotContains(handlerLog, "looking for value", "raw parse-error text must not appear in an enclave-session log")
		assert.Contains(handlerLog, "error hmac:", "the redacted error digest marker must be present")
	})

	t.Run("valid arguments under enclave session take the redacted request/response log branches", func(t *testing.T) {
		assert := assert.New(t)
		require := require.New(t)

		ctx := mcp.WithEnclaveSession(context.WithValue(context.Background(), SessionIDContextKey, "redact-success-session"))
		req := &sdk.CallToolRequest{
			Params: &sdk.CallToolParamsRaw{
				Name:      "redaction-backend___echo",
				Arguments: json.RawMessage(`{"message":"top-secret-request-payload"}`),
			},
		}
		var result *sdk.CallToolResult
		var data interface{}
		var err error
		logOutput := captureServerLog(t, func() {
			result, data, err = echoTool.Handler(ctx, req, nil)
		})
		handlerLog := toolCallLogLines(logOutput)
		require.NotEmpty(handlerLog, "expected at least one tool-call log line; log message text may have drifted from toolCallLogLines' filter")
		require.NoError(err)
		require.NotNil(result)
		assert.False(result.IsError)
		assert.NotNil(data)
		assert.NotContains(handlerLog, "top-secret-request-payload", "raw enclave request payload must not appear in the log")
		assert.NotContains(handlerLog, "echoed-response-sentinel-value", "raw enclave response payload must not appear in the log")
		assert.Contains(handlerLog, "[REDACTED enclave payload", "the redacted payload marker must be present")
	})

	t.Run("backend error under enclave session takes the redacted error-log branch", func(t *testing.T) {
		assert := assert.New(t)
		require := require.New(t)

		ctx := mcp.WithEnclaveSession(context.WithValue(context.Background(), SessionIDContextKey, "redact-error-session"))
		req := &sdk.CallToolRequest{
			Params: &sdk.CallToolParamsRaw{
				Name:      "redaction-backend___boom",
				Arguments: json.RawMessage(`{}`),
			},
		}
		var result *sdk.CallToolResult
		var data interface{}
		var err error
		logOutput := captureServerLog(t, func() {
			result, data, err = boomTool.Handler(ctx, req, nil)
		})
		handlerLog := toolCallLogLines(logOutput)
		require.NotEmpty(handlerLog, "expected at least one tool-call log line; log message text may have drifted from toolCallLogLines' filter")
		require.Error(err)
		require.NotNil(result)
		assert.True(result.IsError)
		assert.Nil(data)
		assert.NotContains(handlerLog, "simulated backend failure", "raw backend error text must not appear in an enclave-session log")
		assert.Contains(handlerLog, "error hmac:", "the redacted error digest marker must be present")
	})

	t.Run("backend error without enclave session takes the non-redacted error-log branch", func(t *testing.T) {
		assert := assert.New(t)
		require := require.New(t)

		ctx := context.WithValue(context.Background(), SessionIDContextKey, "plain-error-session")
		req := &sdk.CallToolRequest{
			Params: &sdk.CallToolParamsRaw{
				Name:      "redaction-backend___boom",
				Arguments: json.RawMessage(`{}`),
			},
		}
		var result *sdk.CallToolResult
		var data interface{}
		var err error
		logOutput := captureServerLog(t, func() {
			result, data, err = boomTool.Handler(ctx, req, nil)
		})
		handlerLog := toolCallLogLines(logOutput)
		require.NotEmpty(handlerLog, "expected at least one tool-call log line; log message text may have drifted from toolCallLogLines' filter")
		require.Error(err)
		require.NotNil(result)
		assert.True(result.IsError)
		assert.Nil(data)
		assert.Contains(handlerLog, "simulated backend failure", "non-enclave sessions must log the raw backend error text")
	})

	t.Run("global payload redaction flag also takes the redacted response-log branch", func(t *testing.T) {
		assert := assert.New(t)
		require := require.New(t)

		previousRedaction := sanitize.PayloadRedactionEnabled()
		sanitize.SetPayloadRedaction(true)
		t.Cleanup(func() { sanitize.SetPayloadRedaction(previousRedaction) })

		ctx := context.WithValue(context.Background(), SessionIDContextKey, "global-redaction-session")
		req := &sdk.CallToolRequest{
			Params: &sdk.CallToolParamsRaw{
				Name:      "redaction-backend___echo",
				Arguments: json.RawMessage(`{"message":"top-secret-global-payload"}`),
			},
		}
		var result *sdk.CallToolResult
		var data interface{}
		var err error
		logOutput := captureServerLog(t, func() {
			result, data, err = echoTool.Handler(ctx, req, nil)
		})
		handlerLog := toolCallLogLines(logOutput)
		require.NotEmpty(handlerLog, "expected at least one tool-call log line; log message text may have drifted from toolCallLogLines' filter")
		require.NoError(err)
		require.NotNil(result)
		assert.False(result.IsError)
		assert.NotNil(data)
		assert.NotContains(handlerLog, "top-secret-global-payload", "raw request payload must not appear in the log when the global redaction flag is set")
		assert.NotContains(handlerLog, "echoed-response-sentinel-value", "raw response payload must not appear in the log when the global redaction flag is set")
		assert.Contains(handlerLog, "[REDACTED enclave payload", "the redacted payload marker must be present")
	})
}

// TestRegisterToolsFromBackend_ToolResponseFilterWrapsHandler verifies that
// when a server config sets a non-empty ToolResponseFilters entry for a
// registered tool, registerToolsFromBackendContext wraps the raw handler with
// middleware.WrapToolHandlerWithFilter (rather than the plain
// middleware.WrapToolHandler used when no filter is configured). This
// exercises the previously-uncovered `filter != ""` branch in
// registerToolsFromBackendContext (internal/server/tool_registry.go) and
// confirms the jq filter is actually applied to the backend's tool result.
func TestRegisterToolsFromBackend_ToolResponseFilterWrapsHandler(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		method, _ := req["method"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]interface{}{},
					"serverInfo":      map[string]interface{}{"name": "filter-backend", "version": "1.0"},
				},
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]interface{}{
					"tools": []map[string]interface{}{
						{
							"name":        "search",
							"description": "search tool",
							"inputSchema": map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
						},
					},
				},
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]interface{}{
					"content": []map[string]interface{}{{"type": "text", "text": "raw result"}},
					"items":   []string{"a", "b", "c"},
				},
			})
		}
	}))
	defer backend.Close()

	cfg := &config.Config{
		Servers: map[string]*config.ServerConfig{
			"filter-backend": {
				Type:                "http",
				URL:                 backend.URL,
				ToolResponseFilters: map[string]string{"search": ".items"},
			},
		},
	}

	us, err := NewUnified(context.Background(), cfg)
	require.NoError(t, err)
	defer us.Close()

	require.NoError(t, us.registerToolsFromBackend("filter-backend"))

	us.toolsMu.RLock()
	tool := us.tools["filter-backend___search"]
	us.toolsMu.RUnlock()
	require.NotNil(t, tool)
	require.NotNil(t, tool.Handler)

	ctx := context.WithValue(context.Background(), SessionIDContextKey, "filter-session")
	req := &sdk.CallToolRequest{
		Params: &sdk.CallToolParamsRaw{
			Name:      "filter-backend___search",
			Arguments: json.RawMessage(`{}`),
		},
	}
	result, data, err := tool.Handler(ctx, req, nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	// The jq filter ".items" must replace the raw backend result (whose
	// "content" text was "raw result") with just the "items" array.
	dataJSON, marshalErr := json.Marshal(data)
	require.NoError(t, marshalErr)
	assert.JSONEq(t, `["a","b","c"]`, string(dataJSON))
}

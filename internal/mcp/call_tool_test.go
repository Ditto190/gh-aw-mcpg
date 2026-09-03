package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCallToolTestBackend spins up a real SDK streamable HTTP MCP server with a
// single tool ("echo_tool") that records the arguments it receives via
// req.Params.Arguments (the SDK-native path, not the gateway's own JSON
// parsing). This lets tests exercise Connection.callTool's real SDK dispatch
// (via callSDKMethod → callParamMethod → sdk.ClientSession.CallTool) end to
// end, rather than mocking at the HTTP JSON-RPC layer.
func newCallToolTestBackend(t *testing.T) (*httptest.Server, *sync.Map) {
	t.Helper()
	received := &sync.Map{} // toolName -> map[string]any arguments

	impl := &sdk.Implementation{Name: "call-tool-test-backend", Version: "1.0.0"}
	mcpServer := sdk.NewServer(impl, nil)
	mcpServer.AddTool(&sdk.Tool{
		Name:        "echo_tool",
		Description: "Echoes back the arguments it receives",
		InputSchema: map[string]interface{}{"type": "object"},
	}, func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		args, err := ParseToolArguments(req)
		if err != nil {
			return &sdk.CallToolResult{
				IsError: true,
				Content: []sdk.Content{&sdk.TextContent{Text: err.Error()}},
			}, nil
		}
		received.Store("echo_tool", args)
		return &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: "ok"}},
		}, nil
	})
	mcpServer.AddTool(&sdk.Tool{
		Name:        "failing_tool",
		Description: "Always returns a Go error from the handler",
		InputSchema: map[string]interface{}{"type": "object"},
	}, func(_ context.Context, _ *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return nil, assert.AnError
	})

	handler := sdk.NewStreamableHTTPHandler(func(_ *http.Request) *sdk.Server {
		return mcpServer
	}, &sdk.StreamableHTTPOptions{Stateless: false})

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.Handle("/mcp/", handler)

	return httptest.NewServer(mux), received
}

// TestCallTool_NilArgumentsDefaultsToEmptyMap verifies that callTool (invoked
// via the real SDK dispatch path) defaults a nil Arguments map to an empty
// map before forwarding the call to the backend, per the MCP protocol
// requirement that "arguments" always be present.
func TestCallTool_NilArgumentsDefaultsToEmptyMap(t *testing.T) {
	srv, received := newCallToolTestBackend(t)
	defer srv.Close()

	conn, err := NewHTTPConnection(context.Background(), "test-server", srv.URL+"/mcp", nil, nil, "", 0, 0)
	require.NoError(t, err)
	defer conn.Close()

	// Explicitly omit "arguments" so p.Arguments unmarshals to nil.
	params := map[string]interface{}{"name": "echo_tool"}
	resp, err := conn.SendRequestWithServerID(context.Background(), "tools/call", params, "test-server")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Nil(t, resp.Error)

	val, ok := received.Load("echo_tool")
	require.True(t, ok, "backend should have recorded the call")
	args, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Empty(t, args, "nil arguments should be normalized to an empty map")
}

// TestCallTool_ArgumentsForwarded verifies that when arguments are provided,
// callTool forwards them unmodified to the backend through the real SDK
// dispatch path.
func TestCallTool_ArgumentsForwarded(t *testing.T) {
	srv, received := newCallToolTestBackend(t)
	defer srv.Close()

	conn, err := NewHTTPConnection(context.Background(), "test-server", srv.URL+"/mcp", nil, nil, "", 0, 0)
	require.NoError(t, err)
	defer conn.Close()

	params := map[string]interface{}{
		"name": "echo_tool",
		"arguments": map[string]interface{}{
			"query": "hello",
			"count": float64(3),
		},
	}
	resp, err := conn.SendRequestWithServerID(context.Background(), "tools/call", params, "test-server")
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Nil(t, resp.Error)

	val, ok := received.Load("echo_tool")
	require.True(t, ok)
	args, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hello", args["query"])
	assert.InEpsilon(t, 3.0, args["count"], 1e-9)
}

// TestCallTool_BackendHandlerError verifies that when the backend tool
// handler returns a Go error (not a CallToolResult with IsError), callTool
// surfaces that as an error through the real SDK dispatch path.
func TestCallTool_BackendHandlerError(t *testing.T) {
	srv, _ := newCallToolTestBackend(t)
	defer srv.Close()

	conn, err := NewHTTPConnection(context.Background(), "test-server", srv.URL+"/mcp", nil, nil, "", 0, 0)
	require.NoError(t, err)
	defer conn.Close()

	params := map[string]interface{}{"name": "failing_tool"}
	resp, err := conn.SendRequestWithServerID(context.Background(), "tools/call", params, "test-server")
	require.Error(t, err)
	assert.Nil(t, resp)
}

// TestCallTool_UnknownToolName verifies that calling a tool name unknown to
// the backend surfaces an error via the real SDK dispatch path (exercises the
// non-nil-session branch differently from the "unsupported method" cases
// covered elsewhere).
func TestCallTool_UnknownToolName(t *testing.T) {
	srv, _ := newCallToolTestBackend(t)
	defer srv.Close()

	conn, err := NewHTTPConnection(context.Background(), "test-server", srv.URL+"/mcp", nil, nil, "", 0, 0)
	require.NoError(t, err)
	defer conn.Close()

	params := map[string]interface{}{"name": "does_not_exist"}
	resp, err := conn.SendRequestWithServerID(context.Background(), "tools/call", params, "test-server")
	require.Error(t, err)
	assert.Nil(t, resp)
}

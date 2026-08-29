package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stdioServerEnvVar signals to the test binary (when re-executed as a
// subprocess by NewConnection) that it should run as a minimal MCP server
// over stdio instead of running the normal test suite. This lets
// TestNewConnection_Success exercise the real success path (handshake +
// Connection struct assembly) without depending on Docker or an external
// MCP server binary.
const stdioServerEnvVar = "GO_TEST_MCP_STDIO_SERVER"

// TestMain re-execs the test binary itself as a stdio MCP server when
// stdioServerEnvVar is set, otherwise it runs the package's tests as usual.
func TestMain(m *testing.M) {
	if os.Getenv(stdioServerEnvVar) == "1" {
		runStdioTestServer()
		return
	}
	os.Exit(m.Run())
}

// runStdioTestServer starts a minimal SDK MCP server bound to stdin/stdout.
// It never returns to the caller; the process exits once the transport closes.
func runStdioTestServer() {
	impl := &sdk.Implementation{Name: "conn-success-test-server", Version: "1.0.0"}
	server := sdk.NewServer(impl, nil)
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// TestNewConnection_Success exercises the success path of NewConnection: it
// re-execs the current test binary as a subprocess (with stdioServerEnvVar
// set) so that a real MCP handshake happens over stdin/stdout, then verifies
// the returned Connection is fully populated and usable.
func TestNewConnection_Success(t *testing.T) {
	selfExe, err := os.Executable()
	require.NoError(t, err, "must be able to locate the running test binary to self-exec as an MCP stdio server")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	env := map[string]string{stdioServerEnvVar: "1"}
	conn, err := NewConnection(ctx, "success-server", selfExe, []string{"-test.run=^$"}, env)
	require.NoError(t, err, "NewConnection should succeed against a real stdio MCP server")
	require.NotNil(t, conn)
	t.Cleanup(func() { _ = conn.Close() })

	assert.False(t, conn.IsHTTP(), "stdio connection should not be marked as HTTP")
	assert.NotNil(t, conn.getSDKSession(), "successful stdio connection should have an active SDK session")
}

// TestNewHTTPConnection_SSEFallbackSuccess exercises the SSE fallback branch
// of NewHTTPConnection: a backend that only serves the deprecated SSE
// transport (not streamable HTTP) should still be connected to successfully,
// with the SSE deprecation warning path and success return covered.
func TestNewHTTPConnection_SSEFallbackSuccess(t *testing.T) {
	impl := &sdk.Implementation{Name: "sse-only-test-server", Version: "1.0.0"}
	mcpServer := sdk.NewServer(impl, nil)

	mux := http.NewServeMux()
	sseHandler := sdk.NewSSEHandler(func(_ *http.Request) *sdk.Server { return mcpServer }, nil)
	mux.Handle("/sse", sseHandler)
	testServer := httptest.NewServer(mux)
	t.Cleanup(testServer.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := NewHTTPConnection(ctx, "sse-server", testServer.URL+"/sse", nil, nil, "", 0, 2*time.Second)
	require.NoError(t, err, "NewHTTPConnection should fall back to SSE and succeed")
	require.NotNil(t, conn)
	t.Cleanup(func() { _ = conn.Close() })

	assert.True(t, conn.IsHTTP())
}

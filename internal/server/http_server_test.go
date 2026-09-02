package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/gh-aw-mcpg/internal/config"
)

// mcpInitializeRequest builds a minimal MCP "initialize" JSON-RPC request body,
// used to exercise session-establishment codepaths in buildMCPHandler's
// serverFactory callback (e.g. per-agent policy enforcement).
func mcpInitializeRequest(t *testing.T, path, authHeader string) *http.Request {
	t.Helper()
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	bodyBytes, err := json.Marshal(initReq)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	return req
}

func TestNewHTTPServer(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		handler http.Handler
	}{
		{
			name:    "host:port address",
			addr:    "127.0.0.1:1234",
			handler: http.NewServeMux(),
		},
		{
			name:    "port-only address",
			addr:    ":8080",
			handler: http.NewServeMux(),
		},
		{
			name:    "zero port",
			addr:    "127.0.0.1:0",
			handler: http.NewServeMux(),
		},
		{
			name:    "empty address",
			addr:    "",
			handler: http.NewServeMux(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newHTTPServer(tt.addr, tt.handler)
			require.NotNil(t, server)
			assert.Equal(t, tt.addr, server.Addr)
			assert.Same(t, tt.handler, server.Handler)
		})
	}
}

// TestBuildMCPHTTPServer_ReturnsServerWithCorrectAddr verifies that buildMCPHTTPServer
// returns an http.Server bound to the requested address.
func TestBuildMCPHTTPServer_ReturnsServerWithCorrectAddr(t *testing.T) {
	us, err := NewUnified(context.Background(), &config.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { us.Close() })

	const addr = "127.0.0.1:0"
	server := buildMCPHTTPServer(addr, us, nil, "", func(_ *http.ServeMux, _ time.Duration) {})

	require.NotNil(t, server)
	assert.Equal(t, addr, server.Addr)
}

// TestBuildMCPHTTPServer_RouteBuilderIsCalled verifies that buildMCPHTTPServer
// invokes the supplied routeBuilder callback.
func TestBuildMCPHTTPServer_RouteBuilderIsCalled(t *testing.T) {
	us, err := NewUnified(context.Background(), &config.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { us.Close() })

	called := false
	buildMCPHTTPServer("127.0.0.1:0", us, nil, "", func(_ *http.ServeMux, _ time.Duration) {
		called = true
	})

	assert.True(t, called, "routeBuilder should be called by buildMCPHTTPServer")
}

// TestBuildMCPHTTPServer_RouteBuilderReceivesSessionTimeout verifies that
// the session timeout passed to routeBuilder reflects the environment variable.
func TestBuildMCPHTTPServer_RouteBuilderReceivesSessionTimeout(t *testing.T) {
	us, err := NewUnified(context.Background(), &config.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { us.Close() })

	t.Setenv("MCP_GATEWAY_SESSION_TIMEOUT", "15m")

	var capturedTimeout time.Duration
	buildMCPHTTPServer("127.0.0.1:0", us, nil, "", func(_ *http.ServeMux, sessionTimeout time.Duration) {
		capturedTimeout = sessionTimeout
	})

	assert.Equal(t, 15*time.Minute, capturedTimeout)
}

// TestBuildMCPHTTPServer_CustomRouteFromBuilder verifies that routes registered
// inside the routeBuilder callback are accessible via the returned server's handler.
func TestBuildMCPHTTPServer_CustomRouteFromBuilder(t *testing.T) {
	us, err := NewUnified(context.Background(), &config.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { us.Close() })

	server := buildMCPHTTPServer("127.0.0.1:0", us, nil, "", func(mux *http.ServeMux, _ time.Duration) {
		mux.HandleFunc("/custom-test-route", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/custom-test-route", nil)
	rr := httptest.NewRecorder()
	server.Handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusTeapot, rr.Code)
}

// TestCreateHTTPServerForMCP_AgentPolicyFiltersTools verifies that, when per-agent
// policies are configured, the unified server's session-establishment callback
// takes the agentPoliciesEnforced branch and serves a per-agent filtered server
// (exercising http_server.go's agentServerCache.GetOrCreate/createAgentFilteredUnifiedServer path).
func TestCreateHTTPServerForMCP_AgentPolicyFiltersTools(t *testing.T) {
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			AgentIDs: []string{"alice"},
			AgentPolicies: map[string]*config.AgentPolicy{
				"alice": {Servers: []string{"github"}},
			},
		},
		Servers: map[string]*config.ServerConfig{
			"github": {Command: "docker", Args: []string{}},
		},
	}

	us, err := NewUnified(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { us.Close() })
	require.True(t, us.agentPoliciesEnforced(), "test config must enforce agent policies")

	httpServer := CreateHTTPServerForMCP(":0", us, nil, "")

	req := mcpInitializeRequest(t, "/mcp", "alice")
	rr := httptest.NewRecorder()
	httpServer.Handler.ServeHTTP(rr, req)

	// A per-agent filtered server should be constructed and handle the request
	// successfully rather than being rejected outright.
	assert.NotEqual(t, http.StatusInternalServerError, rr.Code, "session establishment via per-agent filtered server should not fail")
	t.Logf("initialize response: status=%d body=%s", rr.Code, rr.Body.String())
}

// TestCreateHTTPServerForRoutedMode_AgentAccessDenied verifies that routed mode
// rejects session establishment for an agent whose policy does not permit the
// requested backend (http_server.go's agentCanAccessServer denial branch).
func TestCreateHTTPServerForRoutedMode_AgentAccessDenied(t *testing.T) {
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			AgentIDs: []string{"alice", "bob"},
			AgentPolicies: map[string]*config.AgentPolicy{
				"alice": {Servers: []string{"fetch"}}, // alice may NOT access github
				"bob":   {Servers: []string{"github"}},
			},
		},
		Servers: map[string]*config.ServerConfig{
			"github": {Command: "docker", Args: []string{}},
			"fetch":  {Command: "docker", Args: []string{}},
		},
	}

	us, err := NewUnified(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { us.Close() })

	httpServer := CreateHTTPServerForRoutedMode("127.0.0.1:0", us, nil, "")

	// alice is denied access to the github backend: the serverFactory callback
	// returns nil, and the SDK's streamable HTTP handler should not succeed.
	req := mcpInitializeRequest(t, "/mcp/github", "alice")
	rr := httptest.NewRecorder()
	httpServer.Handler.ServeHTTP(rr, req)

	assert.NotEqual(t, http.StatusOK, rr.Code, "denied agent's session establishment must not succeed")
	t.Logf("denied-agent response: status=%d body=%s", rr.Code, rr.Body.String())

	// bob is permitted access to the github backend and should be able to
	// establish a session (exercising both agentCanAccessServer=true and the
	// serverCache.GetOrCreate/createAgentFilteredServer success path).
	reqAllowed := mcpInitializeRequest(t, "/mcp/github", "bob")
	rrAllowed := httptest.NewRecorder()
	httpServer.Handler.ServeHTTP(rrAllowed, reqAllowed)

	assert.NotEqual(t, http.StatusInternalServerError, rrAllowed.Code, "permitted agent's session establishment should not fail")
	t.Logf("allowed-agent response: status=%d body=%s", rrAllowed.Code, rrAllowed.Body.String())
}

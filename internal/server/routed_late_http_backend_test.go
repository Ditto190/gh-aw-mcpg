package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/gh-aw-mcpg/internal/config"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRoutedHTTPBackendRecoversAfterLateStartup(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(err)
	backendAddress := listener.Addr().String()
	require.NoError(listener.Close())

	const capability = "enclave-test-capability"
	cfg := &config.Config{
		Servers: map[string]*config.ServerConfig{
			"awf-enclave": {
				Type:           "http",
				URL:            "http://" + backendAddress + "/mcp",
				Headers:        map[string]string{"Authorization": "Bearer " + capability},
				Tools:          []string{"enclave_run_script"},
				ConnectTimeout: 1,
				ToolTimeout:    150,
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	us, err := NewUnified(ctx, cfg)
	require.NoError(err, "gateway startup must survive an unavailable HTTP backend")
	defer us.Close()
	assert.Empty(us.GetToolsForBackend("awf-enclave"))

	gateway := CreateHTTPServerForRoutedMode("127.0.0.1:0", us, "", "")
	gatewayServer := httptest.NewServer(gateway.Handler)
	defer gatewayServer.Close()
	route := gatewayServer.URL + "/mcp/awf-enclave"

	status, body := postRawMCP(t, route, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"readiness","version":"1.0"}}}`)
	assert.Equal(http.StatusServiceUnavailable, status)
	assert.NotContains(string(body), capability)

	var (
		receivedMu      sync.Mutex
		receivedHeaders []string
		receivedMethods []string
	)
	backendSDK := sdk.NewServer(&sdk.Implementation{Name: "awf-enclave-mcp", Version: "1.0"}, nil)
	addTestBackendTool(backendSDK, "enclave_run_script")
	addTestBackendTool(backendSDK, "enclave_run_agent")
	backendMCPHandler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server {
		return backendSDK
	}, nil)
	backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := readAndRestoreRequestBody(r)
		if readErr != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var request struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &request)

		receivedMu.Lock()
		receivedHeaders = append(receivedHeaders, r.Header.Get("Authorization"))
		if request.Method != "" {
			receivedMethods = append(receivedMethods, request.Method)
		}
		receivedMu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+capability {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		backendMCPHandler.ServeHTTP(w, r)
	})

	backendListener, err := net.Listen("tcp", backendAddress)
	require.NoError(err)
	backendHTTPServer := &http.Server{Handler: backendHandler}
	backendDone := make(chan error, 1)
	go func() {
		backendDone <- backendHTTPServer.Serve(backendListener)
	}()
	t.Cleanup(func() {
		require.NoError(backendHTTPServer.Close())
		require.ErrorIs(<-backendDone, http.ErrServerClosed)
	})

	time.Sleep(backendRegistrationRetryInterval + 100*time.Millisecond)

	client := sdk.NewClient(&sdk.Implementation{Name: "firewall-readiness", Version: "1.0"}, nil)
	gatewayClient := gatewayServer.Client()
	gatewayClient.Transport = testHeaderTransport{
		base:  gatewayClient.Transport,
		key:   "Authorization",
		value: "Bearer firewall-readiness",
	}
	transport := &sdk.StreamableClientTransport{
		Endpoint:             route,
		HTTPClient:           gatewayClient,
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}
	readinessCtx, readinessCancel := context.WithTimeout(ctx, 10*time.Second)
	defer readinessCancel()
	session, err := client.Connect(readinessCtx, transport, nil)
	require.NoError(err, "initialize and notifications/initialized must succeed after backend startup")
	defer session.Close()

	tools, err := session.ListTools(readinessCtx, &sdk.ListToolsParams{})
	require.NoError(err)
	require.Len(tools.Tools, 1, "the routed tool list must enforce the configured allowlist")
	assert.Equal("enclave_run_script", tools.Tools[0].Name)

	_, err = session.CallTool(readinessCtx, &sdk.CallToolParams{
		Name:      "enclave_run_agent",
		Arguments: map[string]interface{}{},
	})
	require.Error(err, "a tool excluded by the allowlist must not be callable through the routed server")

	receivedMu.Lock()
	defer receivedMu.Unlock()
	require.NotEmpty(receivedHeaders)
	for _, header := range receivedHeaders {
		assert.Equal("Bearer "+capability, header)
	}
	assert.Contains(receivedMethods, "initialize")
	assert.Contains(receivedMethods, "notifications/initialized")
	assert.Contains(receivedMethods, "tools/list")
}

type testHeaderTransport struct {
	base       http.RoundTripper
	key, value string
}

func (t testHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	cloned.Header.Set(t.key, t.value)
	return t.base.RoundTrip(cloned)
}

func addTestBackendTool(server *sdk.Server, name string) {
	server.AddTool(&sdk.Tool{
		Name:        name,
		Description: name,
		InputSchema: map[string]interface{}{"type": "object"},
	}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "ok"}}}, nil
	})
}

func postRawMCP(t *testing.T, url, payload string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var parsed map[string]interface{}
	if resp.StatusCode == http.StatusServiceUnavailable {
		require.NoError(t, json.Unmarshal(body, &parsed))
	}
	return resp.StatusCode, body
}

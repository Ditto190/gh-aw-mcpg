package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/github/gh-aw-mcpg/internal/delegation"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnifiedDelegationRootCommandWithAgentPolicies(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping binary integration test in short mode")
	}

	binaryPath := findBinary(t)
	backend := createDelegationMockMCPBackend(t)
	defer backend.Close()

	gatewayAddr := freeLoopbackAddr(t)
	controlAddr := freeLoopbackAddr(t)
	statePath := filepath.Join(t.TempDir(), "delegation-state.json")
	capabilityKey := "control-capability-key-32-bytes!!"
	envelopeJSON := delegationEnvelopeJSON(t)

	cmd, stdout, stderr := startDelegationGateway(t, binaryPath, gatewayAddr, controlAddr, statePath, capabilityKey, envelopeJSON, backend.URL+"/mcp")
	defer stopCommand(t, cmd)

	serverURL := "http://" + gatewayAddr
	if !waitForServer(t, serverURL+"/health", 10*time.Second) {
		t.Logf("STDOUT: %s", stdout.String())
		t.Logf("STDERR: %s", stderr.String())
		t.Fatal("Server did not start in time")
	}

	status := postDelegationControl(t, controlAddr, capabilityKey, "status", map[string]any{
		"run_id":           "run-1",
		"enclave_entry_id": "entry-1",
	})
	assert.Empty(t, status["labelled_handles"])

	reconcile := postDelegationControl(t, controlAddr, capabilityKey, "reconcile", map[string]any{})
	reconciled, ok := reconcile["reconciled"].(bool)
	require.True(t, ok)
	assert.True(t, reconciled)

	created := postDelegationControl(t, controlAddr, capabilityKey, "create-or-confirm", map[string]any{
		"run_id":           "run-1",
		"enclave_backend":  "awf-enclave",
		"enclave_entry_id": "entry-1",
		"invocation_id":    "inv-1",
		"repository":       "github/gh-aw",
		"tool_policy":      delegation.ToolPolicyGitHubRepositoryReadV1,
		"schema_hash":      "sha256:test",
		"requested_ttl":    30,
		"idempotency_key":  "key-1",
	})
	bearer, ok := created["executor_bearer"].(string)
	require.True(t, ok)
	require.NotEmpty(t, bearer)
	handle, ok := created["handle"].(string)
	require.True(t, ok)
	require.NotEmpty(t, handle)

	mcpSessionID := initializeDelegatedMCP(t, serverURL+"/mcp", bearer)
	tools := delegatedMCPRequest(t, serverURL+"/mcp", bearer, mcpSessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	})
	result, ok := tools["result"].(map[string]any)
	require.True(t, ok, "tools/list response: %#v", tools)
	assert.ElementsMatch(t, []string{"github___issue_read", "github___list_issues"}, toolNamesFromResult(t, result))

	allowed := delegatedMCPRequest(t, serverURL+"/mcp", bearer, mcpSessionID, toolCallPayload(2, "github___list_issues", map[string]any{
		"owner": "github",
		"repo":  "gh-aw",
	}))
	assertDelegatedToolSucceeded(t, allowed)

	wrongRepo := delegatedMCPRequest(t, serverURL+"/mcp", bearer, mcpSessionID, toolCallPayload(3, "github___list_issues", map[string]any{
		"owner": "github",
		"repo":  "sibling",
	}))
	assertDelegatedToolDenied(t, wrongRepo)

	disallowedTool := delegatedMCPRequest(t, serverURL+"/mcp", bearer, mcpSessionID, toolCallPayload(4, "github___search_issues", map[string]any{
		"owner": "github",
		"repo":  "gh-aw",
	}))
	assertDelegatedToolDenied(t, disallowedTool)

	expiring := postDelegationControl(t, controlAddr, capabilityKey, "create-or-confirm", map[string]any{
		"run_id":           "run-1",
		"enclave_backend":  "awf-enclave",
		"enclave_entry_id": "entry-expiring",
		"invocation_id":    "inv-expiring",
		"repository":       "github/gh-aw",
		"tool_policy":      delegation.ToolPolicyGitHubRepositoryReadV1,
		"schema_hash":      "sha256:test",
		"requested_ttl":    1,
		"idempotency_key":  "key-expiring",
	})
	expiringBearer := expiring["executor_bearer"].(string)
	expiringSessionID := initializeDelegatedMCP(t, serverURL+"/mcp", expiringBearer)
	time.Sleep(1100 * time.Millisecond)
	expiredReplay := delegatedMCPRequest(t, serverURL+"/mcp", expiringBearer, expiringSessionID, toolCallPayload(5, "github___list_issues", map[string]any{
		"owner": "github",
		"repo":  "gh-aw",
	}))
	assertDelegatedToolDenied(t, expiredReplay)

	prompts := delegatedMCPRequest(t, serverURL+"/mcp", bearer, mcpSessionID, map[string]any{
		"jsonrpc": "2.0",
		"id":      6,
		"method":  "prompts/list",
	})
	assert.Contains(t, prompts, "error", "delegated sessions must not expose non-tool MCP capabilities")

	postDelegationControl(t, controlAddr, capabilityKey, "revoke", map[string]any{"handle": handle})
	replayed := delegatedMCPRequest(t, serverURL+"/mcp", bearer, mcpSessionID, toolCallPayload(7, "github___list_issues", map[string]any{
		"owner": "github",
		"repo":  "gh-aw",
	}))
	assertDelegatedToolDenied(t, replayed)

	createdForLabels := postDelegationControl(t, controlAddr, capabilityKey, "create-or-confirm", map[string]any{
		"run_id":           "run-1",
		"enclave_backend":  "awf-enclave",
		"enclave_entry_id": "entry-1",
		"invocation_id":    "inv-2",
		"repository":       "github/gh-aw",
		"tool_policy":      delegation.ToolPolicyGitHubRepositoryReadV1,
		"schema_hash":      "sha256:test",
		"requested_ttl":    30,
		"idempotency_key":  "key-2",
	})
	labelBearer := createdForLabels["executor_bearer"].(string)
	labelSessionID := initializeDelegatedMCP(t, serverURL+"/mcp", labelBearer)
	revokedByLabels := postDelegationControl(t, controlAddr, capabilityKey, "revoke-by-labels", map[string]any{
		"run_id":           "run-1",
		"enclave_entry_id": "entry-1",
	})
	revoked, ok := revokedByLabels["revoked"].(float64)
	require.True(t, ok)
	assert.Equal(t, 1, int(revoked))
	labelReplay := delegatedMCPRequest(t, serverURL+"/mcp", labelBearer, labelSessionID, toolCallPayload(8, "github___issue_read", map[string]any{
		"owner": "github",
		"repo":  "gh-aw",
	}))
	assertDelegatedToolDenied(t, labelReplay)

	stopCommand(t, cmd)
	cmd, stdout, stderr = startDelegationGateway(t, binaryPath, gatewayAddr, controlAddr, statePath, capabilityKey, envelopeJSON, backend.URL+"/mcp")
	defer stopCommand(t, cmd)
	if !waitForServer(t, serverURL+"/health", 10*time.Second) {
		t.Logf("STDOUT: %s", stdout.String())
		t.Logf("STDERR: %s", stderr.String())
		t.Fatal("Restarted server did not start in time")
	}
	recoveredStatus := postDelegationControl(t, controlAddr, capabilityKey, "status", map[string]any{
		"run_id":           "run-1",
		"enclave_entry_id": "entry-1",
	})
	assert.Empty(t, recoveredStatus["labelled_handles"], "revoked delegations must remain revoked after restart")
}

func TestUnifiedDelegationRootCommandRejectsPartialActivation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping binary integration test in short mode")
	}

	binaryPath := findBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath,
		"--config-stdin",
		"--listen", freeLoopbackAddr(t),
		"--unified",
	)
	cmd.Env = append(os.Environ(),
		"MCP_GATEWAY_DELEGATION_ENVELOPE="+delegationEnvelopeJSON(t),
		delegation.EnvControlCapabilityKey+"=control-capability-key-32-bytes!!",
		delegation.EnvControlListenAddr+"="+freeLoopbackAddr(t),
		"MCP_GATEWAY_DELEGATION_GENERATION=1",
	)
	configJSON := map[string]any{
		"mcpServers": map[string]any{
			"github": map[string]any{
				"type": "http",
				"url":  "http://127.0.0.1:1/mcp",
			},
		},
		"gateway": map[string]any{
			"port":    portFromAddr(t, freeLoopbackAddr(t)),
			"domain":  "localhost",
			"agentId": "primary-agent",
		},
	}
	configBytes, err := json.Marshal(configJSON)
	require.NoError(t, err)
	cmd.Stdin = bytes.NewReader(configBytes)
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(output), "must be configured together")
}

func createDelegationMockMCPBackend(t *testing.T) *httptest.Server {
	t.Helper()
	impl := &sdk.Implementation{Name: "delegation-mock-backend", Version: "1.0.0"}
	mcpServer := sdk.NewServer(impl, nil)
	for _, toolName := range []string{"issue_read", "list_issues", "search_issues"} {
		name := toolName
		mcpServer.AddTool(&sdk.Tool{
			Name:        name,
			Description: "mock " + name,
			InputSchema: map[string]any{"type": "object"},
		}, func(_ context.Context, _ *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{
				Content: []sdk.Content{&sdk.TextContent{Text: name + " response"}},
			}, nil
		})
	}
	handler := sdk.NewStreamableHTTPHandler(func(_ *http.Request) *sdk.Server {
		return mcpServer
	}, &sdk.StreamableHTTPOptions{Stateless: false})
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.Handle("/mcp/", handler)
	return httptest.NewServer(mux)
}

func delegationEnvelopeJSON(t *testing.T) string {
	t.Helper()
	envelope := delegation.EnvelopeWire{
		RunID:                 "run-1",
		EnclaveBackend:        "awf-enclave",
		AllowedRepositories:   []string{"github/gh-aw"},
		ToolPolicy:            delegation.ToolPolicyGitHubRepositoryReadV1,
		AllowedSchemaHashes:   []string{"sha256:test"},
		MaxIdentityTTLSeconds: 60,
		ExpiresAt:             time.Now().Add(time.Minute),
	}
	raw, err := json.Marshal(envelope)
	require.NoError(t, err)
	return string(raw)
}

func startDelegationGateway(t *testing.T, binaryPath, gatewayAddr, controlAddr, statePath, capabilityKey, envelopeJSON, backendURL string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, binaryPath,
		"--config-stdin",
		"--listen", gatewayAddr,
		"--unified",
	)
	cmd.Env = append(os.Environ(),
		"MCP_GATEWAY_DELEGATION_ENVELOPE="+envelopeJSON,
		delegation.EnvControlCapabilityKey+"="+capabilityKey,
		"MCP_GATEWAY_DELEGATION_STATE_PATH="+statePath,
		"MCP_GATEWAY_DELEGATION_GENERATION=1",
		delegation.EnvControlListenAddr+"="+controlAddr,
	)
	configJSON := map[string]any{
		"mcpServers": map[string]any{
			"github": map[string]any{
				"type": "http",
				"url":  backendURL,
			},
		},
		"gateway": map[string]any{
			"port":     portFromAddr(t, gatewayAddr),
			"domain":   "localhost",
			"agentIds": []string{"primary-agent", "enclave-agent"},
			"agentPolicies": map[string]any{
				"primary-agent": map[string]any{"servers": []string{"github"}},
				"enclave-agent": map[string]any{
					"servers": []string{"github"},
					"tools":   map[string]any{"github": []string{"list_issues", "issue_read"}},
				},
			},
		},
	}
	configBytes, err := json.Marshal(configJSON)
	require.NoError(t, err)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdin = bytes.NewReader(configBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	require.NoError(t, cmd.Start())
	return cmd, stdout, stderr
}

func portFromAddr(t *testing.T, addr string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	parsed, err := strconv.Atoi(port)
	require.NoError(t, err)
	return parsed
}

func stopCommand(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return listener.Addr().String()
}

func postDelegationControl(t *testing.T, controlAddr, capabilityKey, operation string, payload map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "http://"+controlAddr+delegation.ControlPathPrefix+operation, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Authorization", capabilityKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result), string(body))
	return result
}

func initializeDelegatedMCP(t *testing.T, url, bearer string) string {
	t.Helper()
	_, sessionID := delegatedMCPRequestWithSession(t, url, bearer, "", map[string]any{
		"jsonrpc": "2.0",
		"id":      0,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "delegation-integration-test", "version": "1.0.0"},
		},
	})
	require.NotEmpty(t, sessionID)
	return sessionID
}

func delegatedMCPRequest(t *testing.T, url, bearer, mcpSessionID string, payload map[string]any) map[string]any {
	t.Helper()
	result, _ := delegatedMCPRequestWithSession(t, url, bearer, mcpSessionID, payload)
	return result
}

func delegatedMCPRequestWithSession(t *testing.T, url, bearer, mcpSessionID string, payload map[string]any) (map[string]any, string) {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Authorization", bearer)
	req.Header.Set("X-Agent-ID", "spoofed-agent")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if mcpSessionID != "" {
		req.Header.Set("Mcp-Session-Id", mcpSessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	nextSessionID := resp.Header.Get("Mcp-Session-Id")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	if resp.StatusCode != http.StatusOK {
		return map[string]any{"error": map[string]any{"code": resp.StatusCode, "message": string(body)}}, nextSessionID
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "text/event-stream" {
		return parseSSEResponse(t, string(body)), nextSessionID
	}
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result), string(body))
	return result, nextSessionID
}

func toolCallPayload(id int, name string, arguments map[string]any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": arguments,
		},
	}
}

func toolNamesFromResult(t *testing.T, result map[string]any) []string {
	t.Helper()
	tools, ok := result["tools"].([]any)
	require.True(t, ok)
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]any)
		require.True(t, ok)
		name, ok := toolMap["name"].(string)
		require.True(t, ok)
		names = append(names, name)
	}
	return names
}

func assertDelegatedToolSucceeded(t *testing.T, response map[string]any) {
	t.Helper()
	require.NotContains(t, response, "error")
	result, ok := response["result"].(map[string]any)
	require.True(t, ok)
	assert.NotEqual(t, true, result["isError"])
}

func assertDelegatedToolDenied(t *testing.T, response map[string]any) {
	t.Helper()
	if _, ok := response["error"]; ok {
		return
	}
	result, ok := response["result"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, result["isError"])
}

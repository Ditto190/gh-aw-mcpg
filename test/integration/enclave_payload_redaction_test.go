package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// enclavePrivateSentinel stands in for private repository content returned through an
	// enclave-scoped MCP tool. It must not appear anywhere in the exported artifact tree.
	enclavePrivateSentinel = "SENTINEL-ENCLAVE-PRIVATE-CONTENT"
	// publicCallerSentinel is returned to a non-enclave agent. Its presence proves the
	// redaction is scoped to enclave sessions and that logging is otherwise unchanged.
	publicCallerSentinel = "SENTINEL-PUBLIC-CONTENT"
)

// TestEnclaveSessionPayloadsAbsentFromExportedArtifacts starts the gateway the way a public
// workflow would, drives one enclave-scoped session and one ordinary session through a backend
// that echoes its arguments, and then inspects the entire exported log directory tree.
//
// The enclave sentinel must not appear in any exported file (rpc-messages.jsonl,
// mcp-gateway.log, the per-server logs, gateway.md, tools.json, ...), while the non-enclave
// sentinel must still be logged.
func TestEnclaveSessionPayloadsAbsentFromExportedArtifacts(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping binary integration test in short mode")
	}

	binaryPath := findBinary(t)
	backend := createEchoMCPBackend(t)
	defer backend.Close()

	gatewayAddr := freeLoopbackAddr(t)
	logDir := filepath.Join(t.TempDir(), "mcp-logs")

	configJSON, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			"echo": map[string]any{
				"type": "http",
				"url":  backend.URL + "/mcp",
			},
		},
		"gateway": map[string]any{
			"port":     portFromAddr(t, gatewayAddr),
			"domain":   "localhost",
			"agentIds": []string{"public-agent", "enclave-agent"},
			"agentPolicies": map[string]any{
				"public-agent": map[string]any{"servers": []string{"echo"}},
				"enclave-agent": map[string]any{
					"servers": []string{"echo"},
					"enclave": true,
				},
			},
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath,
		"--config-stdin",
		"--listen", gatewayAddr,
		"--unified",
		"--log-dir", logDir,
	)
	// Public workflows commonly run the gateway with DEBUG=* enabled, and every debug
	// logger also copies its output into the exported file logger. Running the
	// regression in that mode is the only way it covers the configuration whose
	// artifacts are actually published.
	cmd.Env = append(os.Environ(), "DEBUG=*", "DEBUG_COLORS=0")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdin = bytes.NewReader(configJSON)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	require.NoError(t, cmd.Start())
	defer stopCommand(t, cmd)

	serverURL := "http://" + gatewayAddr
	if !waitForServer(t, serverURL+"/health", 20*time.Second) {
		t.Logf("STDOUT: %s", stdout.String())
		t.Logf("STDERR: %s", stderr.String())
		t.Fatal("Gateway did not start in time")
	}

	// The enclave-scoped caller: request arguments and the echoed response both carry the
	// private sentinel.
	enclaveSession := initializeDelegatedMCP(t, serverURL+"/mcp", "enclave-agent")
	enclaveResult := delegatedMCPRequest(t, serverURL+"/mcp", "enclave-agent", enclaveSession,
		toolCallPayload(1, "echo___echo", map[string]any{"message": enclavePrivateSentinel}))
	assert.NotContains(t, fmt.Sprintf("%v", enclaveResult), "error",
		"enclave tool call should succeed: %v", enclaveResult)

	// An ordinary caller on the same gateway.
	publicSession := initializeDelegatedMCP(t, serverURL+"/mcp", "public-agent")
	delegatedMCPRequest(t, serverURL+"/mcp", "public-agent", publicSession,
		toolCallPayload(2, "echo___echo", map[string]any{"message": publicCallerSentinel}))

	stopCommand(t, cmd)

	artifacts := readArtifactTree(t, logDir)
	require.NotEmpty(t, artifacts, "expected exported log artifacts in %s", logDir)

	publicSeen := false
	for name, content := range artifacts {
		assert.NotContains(t, content, enclavePrivateSentinel,
			"exported artifact %s must not contain enclave payload content", name)
		if strings.Contains(content, publicCallerSentinel) {
			publicSeen = true
		}
	}
	assert.True(t, publicSeen, "non-enclave logging behavior must remain unchanged")

	// Enclave traffic must still be diagnosable from metadata alone.
	jsonl, ok := artifacts["rpc-messages.jsonl"]
	require.True(t, ok, "expected rpc-messages.jsonl in %v", artifactNames(artifacts))
	assert.Contains(t, jsonl, `"redacted":true`)
	assert.Contains(t, jsonl, `"tool_name":"echo"`)
}

// readArtifactTree reads every regular file under root, keyed by its path relative to root.
func readArtifactTree(t *testing.T, root string) map[string]string {
	t.Helper()
	artifacts := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		artifacts[relative] = string(content)
		return nil
	})
	require.NoError(t, err)
	return artifacts
}

func artifactNames(artifacts map[string]string) []string {
	names := make([]string, 0, len(artifacts))
	for name := range artifacts {
		names = append(names, name)
	}
	return names
}

// createEchoMCPBackend serves a single tool that echoes its argument back, standing in for a
// backend that returns private repository content to an enclave-scoped caller.
func createEchoMCPBackend(t *testing.T) *httptest.Server {
	t.Helper()
	mcpServer := sdk.NewServer(&sdk.Implementation{Name: "echo-backend", Version: "1.0.0"}, nil)
	mcpServer.AddTool(&sdk.Tool{
		Name:        "echo",
		Description: "Echo the provided message",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"message": map[string]any{"type": "string"}},
		},
	}, func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		var args struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(req.Params.Arguments, &args)
		return &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: args.Message}},
		}, nil
	})
	handler := sdk.NewStreamableHTTPHandler(func(_ *http.Request) *sdk.Server {
		return mcpServer
	}, &sdk.StreamableHTTPOptions{Stateless: false})
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	mux.Handle("/mcp/", handler)
	return httptest.NewServer(mux)
}

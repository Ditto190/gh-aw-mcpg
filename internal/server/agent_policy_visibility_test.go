package server

import (
	"context"
	"testing"
	"time"

	"github.com/github/gh-aw-mcpg/internal/config"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listToolsViaInMemory connects an SDK client to the given server over an
// in-memory transport and returns the tool names it advertises.
func listToolsViaInMemory(server *sdk.Server) ([]string, error) {
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() { _ = server.Run(ctx, serverTransport) }()

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "1.0"}, &sdk.ClientOptions{})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		return nil, err
	}
	defer clientSession.Close()

	listResult, err := clientSession.ListTools(ctx, &sdk.ListToolsParams{})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(listResult.Tools))
	for _, tool := range listResult.Tools {
		names = append(names, tool.Name)
	}
	return names, nil
}

func TestRegisterFilteredTools(t *testing.T) {
	server := newSDKServer("filtered-tools-test", logTransport)
	handler := func(context.Context, *sdk.CallToolRequest, interface{}) (*sdk.CallToolResult, interface{}, error) {
		return &sdk.CallToolResult{}, nil, nil
	}
	tools := []ToolInfo{
		{Name: "allowed", BackendID: "github", InputSchema: map[string]interface{}{"type": "object"}},
		{Name: "denied", BackendID: "github", InputSchema: map[string]interface{}{"type": "object"}},
		{Name: "no-handler", BackendID: "github", InputSchema: map[string]interface{}{"type": "object"}},
	}

	registered := registerFilteredTools(
		server,
		tools,
		"alice",
		func(tool ToolInfo) (string, string) { return tool.BackendID, tool.Name },
		func(_ string, _ string, toolName string) bool { return toolName != "denied" },
		func(tool ToolInfo) func(context.Context, *sdk.CallToolRequest, interface{}) (*sdk.CallToolResult, interface{}, error) {
			if tool.Name == "no-handler" {
				return nil
			}
			return handler
		},
	)

	assert.Equal(t, 1, registered)
	toolsListed, err := listToolsViaInMemory(server)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"allowed"}, toolsListed)
}

func agentVisibilityServer(t *testing.T) *UnifiedServer {
	t.Helper()
	cfg := &config.Config{
		Gateway: &config.GatewayConfig{
			AgentIDs: []string{"alice", "bob"},
			AgentPolicies: map[string]*config.AgentPolicy{
				// alice: github, but only the issue_read tool.
				"alice": {Servers: []string{"github"}, Tools: map[string][]string{"github": {"issue_read"}}},
				// bob: fetch only.
				"bob": {Servers: []string{"fetch"}},
			},
		},
		Servers: map[string]*config.ServerConfig{},
	}

	us, err := NewUnified(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = us.Close() })

	mockHandler := func(ctx context.Context, req *sdk.CallToolRequest, state interface{}) (*sdk.CallToolResult, interface{}, error) {
		return &sdk.CallToolResult{}, state, nil
	}
	us.toolsMu.Lock()
	us.tools["github___issue_read"] = &ToolInfo{Name: "github___issue_read", Description: "Read an issue", BackendID: "github", InputSchema: map[string]interface{}{"type": "object"}, Handler: mockHandler}
	us.tools["github___repo_delete"] = &ToolInfo{Name: "github___repo_delete", Description: "Delete a repo", BackendID: "github", InputSchema: map[string]interface{}{"type": "object"}, Handler: mockHandler}
	us.tools["fetch___get"] = &ToolInfo{Name: "fetch___get", Description: "Fetch URL", BackendID: "fetch", InputSchema: map[string]interface{}{"type": "object"}, Handler: mockHandler}
	us.toolsMu.Unlock()
	return us
}

// TestCreateAgentFilteredServer_RoutedPolicyIsolation verifies that, in routed
// mode, each agent only sees the tools permitted by its own policy on a given
// backend, and that an agent with no access to a backend sees no tools there.
func TestCreateAgentFilteredServer_RoutedPolicyIsolation(t *testing.T) {
	us := agentVisibilityServer(t)

	// alice on github: only issue_read is visible (repo_delete filtered out).
	aliceGithub, err := listToolsViaInMemory(createAgentFilteredServer(us, "github", "alice"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"issue_read"}, aliceGithub)

	// bob has no github access: the github route exposes zero tools to bob.
	bobGithub, err := listToolsViaInMemory(createAgentFilteredServer(us, "github", "bob"))
	require.NoError(t, err)
	assert.Empty(t, bobGithub, "bob must not see any github tools")

	// bob on fetch: sees the fetch tool.
	bobFetch, err := listToolsViaInMemory(createAgentFilteredServer(us, "fetch", "bob"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"get"}, bobFetch)
}

// TestCreateAgentFilteredUnifiedServer_PolicyIsolation verifies that, in unified
// mode, the aggregated (prefixed) tool list is filtered per authenticated agent.
func TestCreateAgentFilteredUnifiedServer_PolicyIsolation(t *testing.T) {
	us := agentVisibilityServer(t)

	aliceTools, err := listToolsViaInMemory(createAgentFilteredUnifiedServer(us, "alice"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"github___issue_read"}, aliceTools, "alice sees only her permitted github tool")

	bobTools, err := listToolsViaInMemory(createAgentFilteredUnifiedServer(us, "bob"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"fetch___get"}, bobTools, "bob sees only fetch")

	// An agent with no policy sees nothing (fail-closed).
	ghostTools, err := listToolsViaInMemory(createAgentFilteredUnifiedServer(us, "ghost"))
	require.NoError(t, err)
	assert.Empty(t, ghostTools, "an agent without a policy sees no tools")
}

// TestCreateAgentFilteredUnifiedServer_ConcurrentIsolation exercises the per-agent
// filtered-server construction concurrently to catch data races (run with -race).
func TestCreateAgentFilteredUnifiedServer_ConcurrentIsolation(t *testing.T) {
	us := agentVisibilityServer(t)

	agents := []string{"alice", "bob", "ghost"}
	expected := map[string][]string{
		"alice": {"github___issue_read"},
		"bob":   {"fetch___get"},
		"ghost": {},
	}
	type result struct {
		agentID string
		tools   []string
		err     error
	}
	done := make(chan result, len(agents)*4)
	for i := 0; i < 4; i++ {
		for _, a := range agents {
			go func(agentID string) {
				tools, err := listToolsViaInMemory(createAgentFilteredUnifiedServer(us, agentID))
				done <- result{agentID: agentID, tools: tools, err: err}
			}(a)
		}
	}
	for i := 0; i < len(agents)*4; i++ {
		result := <-done
		require.NoError(t, result.err)
		assert.ElementsMatch(t, expected[result.agentID], result.tools)
	}
}

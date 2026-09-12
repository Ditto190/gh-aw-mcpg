package server

import (
	"context"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dummyToolHandler is a shared no-op tool handler used across delegation
// filtered-server tests.
func dummyToolHandler(context.Context, *sdk.CallToolRequest, interface{}) (*sdk.CallToolResult, interface{}, error) {
	return &sdk.CallToolResult{}, nil, nil
}

// delegationTestUnifiedServer builds a minimal UnifiedServer whose registered
// tool set spans multiple backends, delegated and non-delegated tool names,
// and one delegated-named tool with a missing handler (to exercise the
// nil-handler skip branch inside registerFilteredTools).
func delegationTestUnifiedServer(t *testing.T) *UnifiedServer {
	t.Helper()
	us := &UnifiedServer{
		tools: map[string]*ToolInfo{
			"github___issue_read": {
				Name:        "github___issue_read",
				BackendID:   "github",
				InputSchema: map[string]interface{}{"type": "object"},
				Handler:     dummyToolHandler,
			},
			"github___list_issues": {
				Name:        "github___list_issues",
				BackendID:   "github",
				InputSchema: map[string]interface{}{"type": "object"},
				Handler:     dummyToolHandler,
			},
			// Non-delegated github tool: must be excluded even though it
			// belongs to the "github" backend.
			"github___delete_repo": {
				Name:        "github___delete_repo",
				BackendID:   "github",
				InputSchema: map[string]interface{}{"type": "object"},
				Handler:     dummyToolHandler,
			},
			// Delegated-named tool on a non-github backend: must be
			// excluded because delegation is github-only.
			"fetch___issue_read": {
				Name:        "fetch___issue_read",
				BackendID:   "fetch",
				InputSchema: map[string]interface{}{"type": "object"},
				Handler:     dummyToolHandler,
			},
		},
	}
	return us
}

// TestCreateDelegationFilteredServer_NonGitHubBackend_ReturnsEmptyServer
// verifies the early-return branch: delegation is only defined for the
// "github" backend, so requesting a filtered server for any other backend
// yields a server with zero registered tools.
func TestCreateDelegationFilteredServer_NonGitHubBackend_ReturnsEmptyServer(t *testing.T) {
	us := delegationTestUnifiedServer(t)

	server := createDelegationFilteredServer(us, "fetch")
	require.NotNil(t, server)

	tools, err := listToolsViaInMemory(server)
	require.NoError(t, err)
	assert.Empty(t, tools)
}

// TestCreateDelegationFilteredServer_GitHubBackend_OnlyDelegatedToolsExposed
// verifies the github-backend branch: only the delegated tool names are
// registered (with the backend prefix stripped by GetToolsForBackend), and
// non-delegated github tools are excluded.
func TestCreateDelegationFilteredServer_GitHubBackend_OnlyDelegatedToolsExposed(t *testing.T) {
	us := delegationTestUnifiedServer(t)

	server := createDelegationFilteredServer(us, "github")
	require.NotNil(t, server)

	tools, err := listToolsViaInMemory(server)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"issue_read", "list_issues"}, tools)
}

// TestCreateDelegationFilteredServer_MissingHandler_ToolSkipped verifies the
// nil-handler branch inside registerFilteredTools: a delegated tool name
// present in the backend's tool list but with no registered handler (because
// GetToolHandler found no match) is silently skipped rather than registered
// with a nil handler.
func TestCreateDelegationFilteredServer_MissingHandler_ToolSkipped(t *testing.T) {
	us := &UnifiedServer{
		tools: map[string]*ToolInfo{
			// issue_read is delegated but intentionally has no matching
			// entry reachable via GetToolHandler("github", "issue_read")
			// because we register it under a different backend prefix key
			// than GetToolsForBackend("github") will report, simulating a
			// tool-list/handler-map mismatch.
			"github___issue_read": {
				Name:        "github___issue_read",
				BackendID:   "github",
				InputSchema: map[string]interface{}{"type": "object"},
				Handler:     dummyToolHandler,
			},
		},
	}

	// Sanity: GetToolHandler resolves normally when the map is consistent.
	require.NotNil(t, us.GetToolHandler("github", "issue_read"))

	// Now break the mapping so GetToolHandler cannot find the handler under
	// the exact prefixed key it looks up, while GetToolsForBackend still
	// reports the tool (its filter is on BackendID, independent of map key).
	us.tools["github___mismatched-key"] = us.tools["github___issue_read"]
	delete(us.tools, "github___issue_read")

	server := createDelegationFilteredServer(us, "github")
	require.NotNil(t, server)

	tools, err := listToolsViaInMemory(server)
	require.NoError(t, err)
	assert.Empty(t, tools, "tool with unresolved handler must be skipped, not registered")
}

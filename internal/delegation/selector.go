// Package delegation implements the github-repository-delegation-v1
// control-plane contract described by github/gh-aw-firewall ADR 0001
// ("Agent enclave repository admission"). It lets AWF create/confirm and
// revoke short-lived, invocation-scoped mcpg identities for dynamically
// admitted agent enclaves without adding, removing, or mutating any
// configured MCP backend, route, or tool.
package delegation

import (
	"slices"
)

// ToolPolicyGitHubRepositoryReadV1 is the only delegated tool policy this
// package understands: a closed allowlist of repository-scoped read-only
// GitHub tools (list_issues and issue_read).
const ToolPolicyGitHubRepositoryReadV1 = "github-repository-read-v1"

// delegatedTools is the fixed, closed set of tools permitted by
// github-repository-read-v1. This set must never grow without a new
// versioned policy name.
var delegatedTools = map[string]struct{}{
	"issue_read":  {},
	"list_issues": {},
}

// DelegatedTools returns the exact, closed set of tools permitted by
// github-repository-read-v1. Each call allocates a fresh slice; callers on a
// hot path (such as per-call authorization checks) should use
// IsDelegatedTool instead.
func DelegatedTools() []string {
	tools := make([]string, 0, len(delegatedTools))
	for tool := range delegatedTools {
		tools = append(tools, tool)
	}
	slices.Sort(tools)
	return tools
}

// IsDelegatedTool reports whether tool is a member of the closed
// github-repository-read-v1 tool set, without allocating.
func IsDelegatedTool(tool string) bool {
	_, ok := delegatedTools[tool]
	return ok
}

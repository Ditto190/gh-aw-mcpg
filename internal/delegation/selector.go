// Package delegation implements the github-repository-delegation-v1
// control-plane contract described by github/gh-aw-firewall ADR 0001
// ("Agent enclave repository admission"). It lets AWF create/confirm and
// revoke short-lived, invocation-scoped mcpg identities for dynamically
// admitted agent enclaves without adding, removing, or mutating any
// configured MCP backend, route, or tool.
package delegation

import (
	"slices"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/reposelector"
)

var logSelector = logger.ForFile()

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

// IsCanonicalRepositorySelector reports whether selector is already the exact
// canonical ASCII byte sequence required by the ADR:
//
//	^[a-z0-9](?:[a-z0-9-]{0,38})/(?!\.\.?$)(?!.*\.\.)[a-z0-9._-]{1,100}$
//
// The grammar itself lives in internal/reposelector, the single source of
// truth shared with the guard-policy and enclave-policy validators; this
// wrapper only adds the per-branch rejection logging delegation admission
// depends on. There is no trimming, case folding, Unicode normalization, URL
// decoding, or alternate syntax: callers must reject any selector for which
// this returns false rather than attempt to normalize it.
func IsCanonicalRepositorySelector(selector string) bool {
	if !isASCII(selector) {
		logSelector.Print("rejected repository selector: non-ASCII bytes present")
		return false
	}
	owner, name, found := strings.Cut(selector, "/")
	if !found || !reposelector.IsCanonicalOwner(owner) {
		logSelector.Print("rejected repository selector: does not match canonical owner/repo pattern")
		return false
	}
	if !reposelector.IsCanonicalRepoName(name) {
		if name == "." || name == ".." || strings.Contains(name, "..") {
			logSelector.Print("rejected repository selector: repo segment is '.', '..', or contains '..'")
		} else {
			logSelector.Print("rejected repository selector: does not match canonical owner/repo pattern")
		}
		return false
	}
	return true
}

// IsCanonicalOwner reports whether selector is already the exact canonical
// ASCII byte sequence required for a repository owner:
// ^[a-z0-9](?:[a-z0-9-]{0,38})$. There is no trimming, case folding, Unicode
// normalization, or URL decoding: callers must reject any selector for which
// this returns false rather than attempt to normalize it.
func IsCanonicalOwner(selector string) bool {
	ok := isASCII(selector) && reposelector.IsCanonicalOwner(selector)
	if !ok {
		logSelector.Print("rejected owner selector: not a canonical ASCII owner segment")
	}
	return ok
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7f {
			return false
		}
	}
	return true
}

package proxy

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// guardSourceRoot is the Rust guard source directory relative to internal/proxy.
const guardSourceRoot = "../../guards/github-guard/rust-guard/src/labels"

// genericRouterToolNames lists router tool names that intentionally do not
// correspond to a GitHub MCP tool recognised by the guard's tool rules. These
// routes fall back to the guard's baseline labelling on purpose.
var genericRouterToolNames = map[string]bool{
	// GraphQL schema introspection carries no repository-scoped data, so the
	// guard has no dedicated rule for it.
	"graphql_introspection": true,
}

var (
	rustConstPattern     = regexp.MustCompile(`pub const ([A-Z0-9_]+): &str = "([^"]+)"`)
	rustToolNamePattern  = regexp.MustCompile(`"([a-z][a-z0-9_]*)"`)
	rustConstRefPattern  = regexp.MustCompile(`tool_names::([A-Z0-9_]+)`)
	goRouterToolsPattern = regexp.MustCompile(`toolName:\s*"([a-z][a-z0-9_]*)"`)
)

// guardRecognizedToolNames extracts the tool names the Rust guard's tool rules
// match on. Literals are collected from match-arm lines in tool_rules.rs,
// resolving tool_names::CONSTANT references through constants.rs.
func guardRecognizedToolNames(t *testing.T) map[string]bool {
	t.Helper()

	constsSrc, err := os.ReadFile(filepath.Join(guardSourceRoot, "constants.rs"))
	require.NoError(t, err, "guard constants.rs must be readable")
	consts := make(map[string]string)
	for _, m := range rustConstPattern.FindAllStringSubmatch(string(constsSrc), -1) {
		consts[m[1]] = m[2]
	}

	rulesSrc, err := os.ReadFile(filepath.Join(guardSourceRoot, "tool_rules.rs"))
	require.NoError(t, err, "guard tool_rules.rs must be readable")

	names := make(map[string]bool)
	for _, line := range strings.Split(string(rulesSrc), "\n") {
		trimmed := strings.TrimSpace(line)
		// Match-arm lines start with a literal, an arm continuation (`|`),
		// or a tool_names:: constant reference.
		if !strings.HasPrefix(trimmed, `"`) && !strings.HasPrefix(trimmed, "|") && !strings.HasPrefix(trimmed, "tool_names::") {
			continue
		}
		for _, m := range rustToolNamePattern.FindAllStringSubmatch(trimmed, -1) {
			names[m[1]] = true
		}
		for _, m := range rustConstRefPattern.FindAllStringSubmatch(trimmed, -1) {
			if val, ok := consts[m[1]]; ok {
				names[val] = true
			}
		}
	}
	return names
}

// TestRouterToolNamesMatchGuardRules verifies that every tool name produced by
// the REST and GraphQL routers is recognised by the Rust guard's tool rules.
// A mismatch (e.g. the plural "list_labels" vs. the guard's "list_label")
// silently degrades responses to baseline labels, which filter mode then
// replaces with an empty result.
func TestRouterToolNamesMatchGuardRules(t *testing.T) {
	guardNames := guardRecognizedToolNames(t)

	// Sanity-check the extraction so a parsing regression cannot make this
	// test vacuously pass.
	require.Greater(t, len(guardNames), 100, "guard tool-name extraction looks broken")
	for _, known := range []string{"list_label", "get_label", "issue_read", "list_issues"} {
		require.True(t, guardNames[known], "expected guard rules to recognise %q", known)
	}

	routerNames := map[string]string{} // tool name → source file
	for _, r := range routes {
		routerNames[r.toolName] = "router.go"
	}
	for _, p := range graphqlPatterns {
		routerNames[p.toolName] = "graphql.go"
	}

	for name, src := range routerNames {
		t.Run(name, func(t *testing.T) {
			if genericRouterToolNames[name] {
				t.Skipf("%s is an intentionally generic route", name)
			}
			assert.True(t, guardNames[name],
				"%s maps a route to tool %q, which the guard tool rules do not recognise", src, name)
		})
	}
}

// TestGoRouterSourceToolNamesAreParsed guards the regexp used by tooling and
// documentation to enumerate router tool names from source.
func TestGoRouterSourceToolNamesAreParsed(t *testing.T) {
	src, err := os.ReadFile("router.go")
	require.NoError(t, err)
	matches := goRouterToolsPattern.FindAllStringSubmatch(string(src), -1)
	require.NotEmpty(t, matches)

	found := make(map[string]bool, len(matches))
	for _, m := range matches {
		found[m[1]] = true
	}
	assert.True(t, found["list_label"], "labels route must use the guard's singular list_label tool name")
	assert.False(t, found["list_labels"], "plural list_labels is not recognised by the guard")
}

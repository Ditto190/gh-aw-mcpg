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
	rustConstPattern    = regexp.MustCompile(`pub const ([A-Z0-9_]+):\s*&str\s*=\s*"([^"]+)"`)
	rustToolNamePattern = regexp.MustCompile(`"([a-z][a-z0-9_]*)"`)
	rustConstRefPattern = regexp.MustCompile(`tool_names::([A-Z0-9_]+)`)

	goRouterToolsPattern = regexp.MustCompile(`toolName:\s*"([a-z][a-z0-9_]*)"`)
)

// rustToolNameConstants parses the `pub const NAME: &str = "value"` tool-name
// constants from the guard's constants.rs. The declaration may wrap across
// lines, so whitespace (including newlines) is tolerated between tokens.
func rustToolNameConstants(t *testing.T) map[string]string {
	t.Helper()

	src, err := os.ReadFile(filepath.Join(guardSourceRoot, "constants.rs"))
	require.NoError(t, err, "guard constants.rs must be readable")

	consts := make(map[string]string)
	for _, m := range rustConstPattern.FindAllStringSubmatch(string(src), -1) {
		consts[m[1]] = m[2]
	}
	return consts
}

// matchArmPatterns returns the source text of each top-level arm pattern of the
// `match tool_name { ... }` expression in apply_tool_labels. Only depth-one
// patterns (the text preceding `=>`) are returned, so nested `match`/`matches!`
// expressions in arm bodies, helper functions, and the `#[cfg(test)]` module
// are excluded.
func matchArmPatterns(src string) []string {
	idx := strings.Index(src, "match tool_name {")
	if idx < 0 {
		return nil
	}
	body := src[idx+len("match tool_name {"):]

	var (
		patterns []string
		buf      strings.Builder
		depth    = 1 // inside the match block
		inArm    bool
	)
	flush := func() {
		if s := strings.TrimSpace(buf.String()); s != "" {
			patterns = append(patterns, s)
		}
		buf.Reset()
	}

	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '"':
			// Consume the string literal (handling escapes).
			j := i + 1
			for j < len(body) {
				if body[j] == '\\' {
					j += 2
					continue
				}
				if body[j] == '"' {
					break
				}
				j++
			}
			if j >= len(body) {
				return patterns
			}
			if depth == 1 && !inArm {
				buf.WriteString(body[i : j+1])
			}
			i = j
			continue
		case c == '/' && i+1 < len(body) && body[i+1] == '/':
			for i < len(body) && body[i] != '\n' {
				i++
			}
			continue
		case c == '{' || c == '(' || c == '[':
			depth++
			continue
		case c == '}' || c == ')' || c == ']':
			depth--
			if depth == 0 {
				// End of the match expression.
				return patterns
			}
			if depth == 1 && inArm {
				inArm = false
			}
			continue
		case c == '=' && i+1 < len(body) && body[i+1] == '>' && depth == 1 && !inArm:
			flush()
			inArm = true
			i++
			continue
		case c == ',' && depth == 1 && inArm:
			inArm = false
			continue
		}
		if depth == 1 && !inArm {
			buf.WriteByte(c)
		}
	}
	return patterns
}

// guardRecognizedToolNames extracts the tool names the Rust guard's tool rules
// match on. Literals are collected from the top-level arms of the
// `match tool_name` expression in tool_rules.rs, resolving tool_names::CONSTANT
// references through constants.rs.
func guardRecognizedToolNames(t *testing.T) map[string]bool {
	t.Helper()

	consts := rustToolNameConstants(t)

	rulesSrc, err := os.ReadFile(filepath.Join(guardSourceRoot, "tool_rules.rs"))
	require.NoError(t, err, "guard tool_rules.rs must be readable")

	patterns := matchArmPatterns(string(rulesSrc))
	require.NotEmpty(t, patterns, "failed to parse match tool_name arms")

	names := make(map[string]bool)
	for _, pattern := range patterns {
		for _, m := range rustToolNamePattern.FindAllStringSubmatch(pattern, -1) {
			names[m[1]] = true
		}
		for _, m := range rustConstRefPattern.FindAllStringSubmatch(pattern, -1) {
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
	// Names that only appear in helper matches, dispatcher method data, or the
	// #[cfg(test)] module must not leak into the recognised set.
	for _, notATool := range []string{"organization", "list_workflow_runs"} {
		require.False(t, guardNames[notATool],
			"%q is not a top-level `match tool_name` arm; extraction is too permissive", notATool)
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

// TestRustToolNameConstantsHandleWrappedDeclarations ensures the constants
// parser tolerates declarations whose value wraps onto the next line, e.g.
// REMOVE_PULL_REQUEST_REVIEW_COMMENT_REACTION in constants.rs.
func TestRustToolNameConstantsHandleWrappedDeclarations(t *testing.T) {
	consts := rustToolNameConstants(t)
	assert.Equal(t, "remove_pull_request_review_comment_reaction",
		consts["REMOVE_PULL_REQUEST_REVIEW_COMMENT_REACTION"],
		"wrapped `pub const NAME: &str =\\n \"value\"` declarations must be parsed")
	assert.Equal(t, "get_issue", consts["GET_ISSUE"])
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

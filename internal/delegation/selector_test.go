package delegation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelegatedToolsIsClosedSet(t *testing.T) {
	t.Parallel()

	tools := DelegatedTools()
	require.Len(t, tools, 2, "expected exactly 2 delegated tools, got %v", tools)

	want := []string{"issue_read", "list_issues"}
	a := assert.New(t)
	a.ElementsMatch(want, tools, "DelegatedTools should match the closed allowlist exactly")

	// DelegatedTools returns a sorted, freshly allocated slice each call.
	a.LessOrEqual(tools[0], tools[1], "expected tools to be sorted")
	tools2 := DelegatedTools()
	require.Len(t, tools2, 2, "expected second DelegatedTools call to also return 2 items")
	tools[0] = "mutated"
	a.NotEqual("mutated", tools2[0], "mutating one returned slice must not affect subsequent calls")
}

func TestIsDelegatedTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tool string
		want bool
	}{
		{"issue_read is delegated", "issue_read", true},
		{"list_issues is delegated", "list_issues", true},
		{"unknown tool is not delegated", "delete_repo", false},
		{"empty tool name is not delegated", "", false},
		{"case-sensitive mismatch is not delegated", "Issue_Read", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsDelegatedTool(tt.tool))
		})
	}
}

func TestToolPolicyGitHubRepositoryReadV1Constant(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "github-repository-read-v1", ToolPolicyGitHubRepositoryReadV1)
}

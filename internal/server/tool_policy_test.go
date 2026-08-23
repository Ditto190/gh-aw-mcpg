package server

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSingularReadTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{
			name:     "get prefix is a singular read tool",
			toolName: "get_issue",
			want:     true,
		},
		{
			name:     "get prefix with long name is a singular read tool",
			toolName: "get_file_contents",
			want:     true,
		},
		{
			name:     "read suffix is a singular read tool",
			toolName: "issue_read",
			want:     true,
		},
		{
			name:     "create prefix is treated as singular",
			toolName: "create_issue",
			want:     true,
		},
		{
			name:     "update prefix is treated as singular",
			toolName: "update_pull_request",
			want:     true,
		},
		{
			name:     "delete prefix is treated as singular",
			toolName: "delete_branch",
			want:     true,
		},
		{
			name:     "read prefix is a singular read tool",
			toolName: "read_file",
			want:     true,
		},
		{
			name:     "arbitrary non-prefixed tool name is singular",
			toolName: "fork_repository",
			want:     true,
		},
		{
			name:     "list prefix is a collection tool",
			toolName: "list_issues",
			want:     false,
		},
		{
			name:     "list prefix with different resource is a collection tool",
			toolName: "list_commits",
			want:     false,
		},
		{
			name:     "list prefix with multi-word resource",
			toolName: "list_pull_requests",
			want:     false,
		},
		{
			name:     "list prefix alone is a collection tool",
			toolName: "list_",
			want:     false,
		},
		{
			name:     "search prefix is a collection tool",
			toolName: "search_code",
			want:     false,
		},
		{
			name:     "search prefix with different resource",
			toolName: "search_issues",
			want:     false,
		},
		{
			name:     "search prefix with multi-word resource",
			toolName: "search_pull_requests",
			want:     false,
		},
		{
			name:     "search prefix alone is a collection tool",
			toolName: "search_",
			want:     false,
		},
		{
			name:     "list without underscore stays singular",
			toolName: "list",
			want:     true,
		},
		{
			name:     "search without underscore stays singular",
			toolName: "search",
			want:     true,
		},
		{
			name:     "listed prefix does not match list underscore",
			toolName: "listed_items",
			want:     true,
		},
		{
			name:     "listing prefix does not match list underscore",
			toolName: "listing_files",
			want:     true,
		},
		{
			name:     "searching prefix does not match search underscore",
			toolName: "searching_code",
			want:     true,
		},
		{
			name:     "get_list contains list but does not start with list underscore",
			toolName: "get_list",
			want:     true,
		},
		{
			name:     "global_search contains search but does not start with search underscore",
			toolName: "global_search",
			want:     true,
		},
		{
			name:     "List underscore with capital L remains singular",
			toolName: "List_issues",
			want:     true,
		},
		{
			name:     "LIST underscore uppercase remains singular",
			toolName: "LIST_ISSUES",
			want:     true,
		},
		{
			name:     "Search underscore with capital S remains singular",
			toolName: "Search_code",
			want:     true,
		},
		{
			name:     "SEARCH underscore uppercase remains singular",
			toolName: "SEARCH_CODE",
			want:     true,
		},
		{
			name:     "empty string defaults singular",
			toolName: "",
			want:     true,
		},
		{
			name:     "single character is singular",
			toolName: "x",
			want:     true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsSingularReadTool(tt.toolName))
		})
	}
}

// TestIsSingularReadTool_DebugLoggingEnabled exercises the debug-log branch
// (logToolPolicy.Enabled()) which is otherwise never taken in normal test
// runs. logToolPolicy is a package-level *logger.Logger whose enabled state
// is computed once at package init time from the DEBUG environment variable
// present when the test binary started, so t.Setenv here cannot retroactively
// flip it. Instead we re-exec this test in a subprocess with DEBUG=* set
// beforehand, verifying the debug branch runs without panicking and produces
// the expected result.
func TestIsSingularReadTool_DebugLoggingEnabled(t *testing.T) {
	if os.Getenv("GO_WANT_DEBUG_SUBPROCESS") == "1" {
		require.True(t, logToolPolicy.Enabled(), "expected logger to be enabled when DEBUG=* is set before process start")
		assert.True(t, IsSingularReadTool("get_issue"))
		assert.False(t, IsSingularReadTool("list_issues"))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestIsSingularReadTool_DebugLoggingEnabled", "-test.v")
	cmd.Env = append(os.Environ(), "GO_WANT_DEBUG_SUBPROCESS=1", "DEBUG=*")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "subprocess output:\n%s", out)
	assert.Contains(t, string(out), "PASS")
}

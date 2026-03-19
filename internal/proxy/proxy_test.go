package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchRoute(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantTool string
		wantArgs map[string]interface{}
		wantNil  bool
	}{
		// Issues
		{
			name:     "list issues",
			path:     "/repos/octocat/hello-world/issues",
			wantTool: "list_issues",
			wantArgs: map[string]interface{}{"owner": "octocat", "repo": "hello-world"},
		},
		{
			name:     "get issue",
			path:     "/repos/octocat/hello-world/issues/42",
			wantTool: "issue_read",
			wantArgs: map[string]interface{}{"owner": "octocat", "repo": "hello-world", "issue_number": "42"},
		},
		{
			name:     "issue comments",
			path:     "/repos/octocat/hello-world/issues/42/comments",
			wantTool: "issue_read",
			wantArgs: map[string]interface{}{"owner": "octocat", "repo": "hello-world", "issue_number": "42", "method": "get_comments"},
		},
		{
			name:     "issue labels",
			path:     "/repos/octocat/hello-world/issues/42/labels",
			wantTool: "issue_read",
			wantArgs: map[string]interface{}{"owner": "octocat", "repo": "hello-world", "issue_number": "42", "method": "get_labels"},
		},

		// Pull Requests
		{
			name:     "list PRs",
			path:     "/repos/github/gh-aw/pulls",
			wantTool: "list_pull_requests",
			wantArgs: map[string]interface{}{"owner": "github", "repo": "gh-aw"},
		},
		{
			name:     "get PR",
			path:     "/repos/github/gh-aw/pulls/123",
			wantTool: "pull_request_read",
			wantArgs: map[string]interface{}{"owner": "github", "repo": "gh-aw", "pullNumber": "123", "method": "get"},
		},
		{
			name:     "PR files",
			path:     "/repos/github/gh-aw/pulls/123/files",
			wantTool: "pull_request_read",
			wantArgs: map[string]interface{}{"owner": "github", "repo": "gh-aw", "pullNumber": "123", "method": "get_files"},
		},
		{
			name:     "PR reviews",
			path:     "/repos/github/gh-aw/pulls/123/reviews",
			wantTool: "pull_request_read",
			wantArgs: map[string]interface{}{"owner": "github", "repo": "gh-aw", "pullNumber": "123", "method": "get_reviews"},
		},

		// Commits
		{
			name:     "list commits",
			path:     "/repos/org/repo/commits",
			wantTool: "list_commits",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo"},
		},
		{
			name:     "get commit",
			path:     "/repos/org/repo/commits/abc123",
			wantTool: "get_commit",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo", "sha": "abc123"},
		},

		// Branches
		{
			name:     "list branches",
			path:     "/repos/org/repo/branches",
			wantTool: "list_branches",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo"},
		},

		// Tags
		{
			name:     "list tags",
			path:     "/repos/org/repo/tags",
			wantTool: "list_tags",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo"},
		},

		// Releases
		{
			name:     "list releases",
			path:     "/repos/org/repo/releases",
			wantTool: "list_releases",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo"},
		},
		{
			name:     "latest release",
			path:     "/repos/org/repo/releases/latest",
			wantTool: "get_latest_release",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo"},
		},
		{
			name:     "release by tag",
			path:     "/repos/org/repo/releases/tags/v1.0.0",
			wantTool: "get_release_by_tag",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo", "tag": "v1.0.0"},
		},

		// Contents
		{
			name:     "file contents",
			path:     "/repos/org/repo/contents/README.md",
			wantTool: "get_file_contents",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo", "path": "README.md"},
		},
		{
			name:     "nested file contents",
			path:     "/repos/org/repo/contents/src/main.go",
			wantTool: "get_file_contents",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo", "path": "src/main.go"},
		},

		// Labels
		{
			name:     "get label",
			path:     "/repos/org/repo/labels/bug",
			wantTool: "get_label",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo", "name": "bug"},
		},

		// Search
		{
			name:     "search code",
			path:     "/search/code",
			wantTool: "search_code",
			wantArgs: map[string]interface{}{},
		},
		{
			name:     "search issues",
			path:     "/search/issues",
			wantTool: "search_issues",
			wantArgs: map[string]interface{}{},
		},

		// User — not mapped; unknown paths are blocked (fail closed)
		{
			name:    "get me",
			path:    "/user",
			wantNil: true,
		},

		// Query string stripping
		{
			name:     "path with query string",
			path:     "/repos/org/repo/issues?state=open&per_page=10",
			wantTool: "list_issues",
			wantArgs: map[string]interface{}{"owner": "org", "repo": "repo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := MatchRoute(tt.path)
			if tt.wantNil {
				assert.Nil(t, match)
				return
			}
			require.NotNil(t, match, "expected route match for %s", tt.path)
			assert.Equal(t, tt.wantTool, match.ToolName)
			assert.Equal(t, tt.wantArgs, match.Args)
		})
	}
}

func TestStripGHHostPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/api/v3/repos/org/repo/issues", "/repos/org/repo/issues"},
		{"/api/v3/user", "/user"},
		{"/api/v3/graphql", "/graphql"},
		{"/repos/org/repo/issues", "/repos/org/repo/issues"},
		{"/user", "/user"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, StripGHHostPrefix(tt.input))
		})
	}
}

func TestMatchGraphQL(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantTool string
		wantNil  bool
	}{
		{
			name:     "issue list query",
			body:     `{"query":"query { repository(owner: \"octocat\", name: \"hello-world\") { issues(first: 10) { nodes { title } } } }"}`,
			wantTool: "list_issues",
		},
		{
			name:     "single issue query",
			body:     `{"query":"query { repository(owner: \"octocat\", name: \"hello-world\") { issue(number: 1) { title body } } }"}`,
			wantTool: "issue_read",
		},
		{
			name:     "PR list query",
			body:     `{"query":"query { repository(owner: \"org\", name: \"repo\") { pullRequests(first: 10) { nodes { title } } } }"}`,
			wantTool: "list_pull_requests",
		},
		{
			name:     "single PR query",
			body:     `{"query":"query { repository(owner: \"org\", name: \"repo\") { pullRequest(number: 1) { title } } }"}`,
			wantTool: "pull_request_read",
		},
		{
			name:     "search query",
			body:     `{"query":"query { search(query: \"is:issue\", type: ISSUE, first: 10) { nodes { ... on Issue { title } } } }"}`,
			wantTool: "search_issues",
		},
		{
			name:    "viewer query",
			body:    `{"query":"query { viewer { login name email } }"}`,
			wantNil: true,
		},
		{
			name:    "empty query",
			body:    `{"query":""}`,
			wantNil: true,
		},
		{
			name:    "invalid JSON",
			body:    `not json`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := MatchGraphQL([]byte(tt.body))
			if tt.wantNil {
				assert.Nil(t, match)
				return
			}
			require.NotNil(t, match, "expected GraphQL match")
			assert.Equal(t, tt.wantTool, match.ToolName)
		})
	}
}

func TestMatchGraphQL_ExtractsOwnerRepo(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantOwner string
		wantRepo  string
	}{
		{
			name:      "inline owner/name",
			body:      `{"query":"query { repository(owner: \"github\", name: \"copilot\") { issues { nodes { title } } } }"}`,
			wantOwner: "github",
			wantRepo:  "copilot",
		},
		{
			name:      "variables owner/name",
			body:      `{"query":"query($owner: String!, $name: String!) { repository(owner: $owner, name: $name) { issues { nodes { title } } } }","variables":{"owner":"github","name":"copilot"}}`,
			wantOwner: "github",
			wantRepo:  "copilot",
		},
		{
			name:      "variables with repo key",
			body:      `{"query":"query { repository(owner: $owner, name: $name) { issues { nodes { title } } } }","variables":{"owner":"org","repo":"myrepo"}}`,
			wantOwner: "org",
			wantRepo:  "myrepo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := MatchGraphQL([]byte(tt.body))
			require.NotNil(t, match)
			assert.Equal(t, tt.wantOwner, match.Owner)
			assert.Equal(t, tt.wantRepo, match.Repo)
		})
	}
}

func TestIsGraphQLPath(t *testing.T) {
	assert.True(t, IsGraphQLPath("/graphql"))
	assert.True(t, IsGraphQLPath("/graphql/"))
	assert.True(t, IsGraphQLPath("/api/v3/graphql"))
	assert.True(t, IsGraphQLPath("/api/v3/graphql/"))
	assert.False(t, IsGraphQLPath("/repos/org/repo"))
	assert.False(t, IsGraphQLPath("/user"))
}

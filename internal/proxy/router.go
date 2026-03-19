package proxy

import (
	"regexp"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/logger"
)

var logRouter = logger.New("proxy:router")

// RouteMatch contains the result of matching a REST API path to a guard tool name.
type RouteMatch struct {
	ToolName string
	Owner    string
	Repo     string
	Args     map[string]interface{} // Arguments to pass to LabelResource
}

// route defines a pattern → tool name mapping.
type route struct {
	pattern  *regexp.Regexp
	toolName string
	// extractArgs is called with submatches to build the args map
	extractArgs func(matches []string) map[string]interface{}
}

// repoArgs builds the standard owner/repo args map.
func repoArgs(owner, repo string) map[string]interface{} {
	return map[string]interface{}{
		"owner": owner,
		"repo":  repo,
	}
}

// routes is the ordered list of REST URL patterns mapped to guard tool names.
// Patterns are tried in order; first match wins.
var routes = []route{
	// Issues
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues/(\d+)/comments$`),
		toolName: "issue_read",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "issue_number": m[3], "method": "get_comments"}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues/(\d+)/labels$`),
		toolName: "issue_read",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "issue_number": m[3], "method": "get_labels"}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues/(\d+)$`),
		toolName: "issue_read",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "issue_number": m[3]}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues$`),
		toolName: "list_issues",
		extractArgs: func(m []string) map[string]interface{} {
			return repoArgs(m[1], m[2])
		},
	},

	// Pull Requests
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/pulls/(\d+)/files$`),
		toolName: "pull_request_read",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "pullNumber": m[3], "method": "get_files"}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/pulls/(\d+)/reviews$`),
		toolName: "pull_request_read",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "pullNumber": m[3], "method": "get_reviews"}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/pulls/(\d+)/comments$`),
		toolName: "pull_request_read",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "pullNumber": m[3], "method": "get_review_comments"}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/pulls/(\d+)$`),
		toolName: "pull_request_read",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "pullNumber": m[3], "method": "get"}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/pulls$`),
		toolName: "list_pull_requests",
		extractArgs: func(m []string) map[string]interface{} {
			return repoArgs(m[1], m[2])
		},
	},

	// Commits
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/commits/([^/]+)$`),
		toolName: "get_commit",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "sha": m[3]}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/commits$`),
		toolName: "list_commits",
		extractArgs: func(m []string) map[string]interface{} {
			return repoArgs(m[1], m[2])
		},
	},

	// Branches and Tags
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/branches$`),
		toolName: "list_branches",
		extractArgs: func(m []string) map[string]interface{} {
			return repoArgs(m[1], m[2])
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/ref/tags/(.+)$`),
		toolName: "get_tag",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "tag": m[3]}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/tags$`),
		toolName: "list_tags",
		extractArgs: func(m []string) map[string]interface{} {
			return repoArgs(m[1], m[2])
		},
	},

	// Releases
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/releases/latest$`),
		toolName: "get_latest_release",
		extractArgs: func(m []string) map[string]interface{} {
			return repoArgs(m[1], m[2])
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/releases/tags/(.+)$`),
		toolName: "get_release_by_tag",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "tag": m[3]}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/releases$`),
		toolName: "list_releases",
		extractArgs: func(m []string) map[string]interface{} {
			return repoArgs(m[1], m[2])
		},
	},

	// Contents
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/contents/(.+)$`),
		toolName: "get_file_contents",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "path": m[3]}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/git/trees/(.+)$`),
		toolName: "get_file_contents",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "path": m[3]}
		},
	},

	// Labels
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/labels/(.+)$`),
		toolName: "get_label",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "name": m[3]}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/labels$`),
		toolName: "list_labels",
		extractArgs: func(m []string) map[string]interface{} {
			return repoArgs(m[1], m[2])
		},
	},

	// Actions (Workflows)
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/workflows$`),
		toolName: "actions_list",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "method": "list_workflows"}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/actions/runs$`),
		toolName: "actions_list",
		extractArgs: func(m []string) map[string]interface{} {
			return map[string]interface{}{"owner": m[1], "repo": m[2], "method": "list_workflow_runs"}
		},
	},

	// Search APIs
	{
		pattern:  regexp.MustCompile(`^/search/code$`),
		toolName: "search_code",
		extractArgs: func(_ []string) map[string]interface{} {
			return map[string]interface{}{}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/search/issues$`),
		toolName: "search_issues",
		extractArgs: func(_ []string) map[string]interface{} {
			return map[string]interface{}{}
		},
	},
	{
		pattern:  regexp.MustCompile(`^/search/repositories$`),
		toolName: "search_repositories",
		extractArgs: func(_ []string) map[string]interface{} {
			return map[string]interface{}{}
		},
	},

	// User API (/user) is intentionally not mapped — it cannot be correctly labeled
	// by the guard (no recognized tool name with equivalent semantics) and may contain
	// private account data (e.g., email). Unknown paths are blocked by the handler.

	// Generic repo-scoped fallback (must be last)
	{
		pattern:  regexp.MustCompile(`^/repos/([^/]+)/([^/]+)(?:/.*)?$`),
		toolName: "get_file_contents",
		extractArgs: func(m []string) map[string]interface{} {
			return repoArgs(m[1], m[2])
		},
	},
}

// MatchRoute matches a REST API path to a guard tool name.
// The path should NOT include the /api/v3 prefix.
func MatchRoute(path string) *RouteMatch {
	// Strip query string
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}

	for _, r := range routes {
		matches := r.pattern.FindStringSubmatch(path)
		if matches != nil {
			args := r.extractArgs(matches)
			m := &RouteMatch{
				ToolName: r.toolName,
				Args:     args,
			}
			if owner, ok := args["owner"].(string); ok {
				m.Owner = owner
			}
			if repo, ok := args["repo"].(string); ok {
				m.Repo = repo
			}
			logRouter.Printf("matched %s → tool=%s owner=%s repo=%s", path, m.ToolName, m.Owner, m.Repo)
			return m
		}
	}

	logRouter.Printf("no route match for %s", path)
	return nil
}

// StripGHHostPrefix removes the /api/v3 prefix that gh adds when using GH_HOST.
func StripGHHostPrefix(path string) string {
	if strings.HasPrefix(path, ghHostPathPrefix) {
		return strings.TrimPrefix(path, ghHostPathPrefix)
	}
	return path
}

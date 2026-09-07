package enclavegithub

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/restroute"
)

var logRoute = logger.ForFile()

var (
	issuesListPath = regexp.MustCompile(`^/repos/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/issues$`)
	issueGetPath   = regexp.MustCompile(`^/repos/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/issues/([1-9][0-9]*)$`)
	commentsPath   = regexp.MustCompile(`^/repos/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/issues/([1-9][0-9]*)/comments$`)
	enclaveRoutes  = []struct {
		pattern   *regexp.Regexp
		operation string
	}{
		{issuesListPath, OperationIssuesList},
		{issueGetPath, OperationIssuesGet},
		{commentsPath, OperationIssueCommentsList},
	}
)

var allowedQueryKeys = map[string]map[string]struct{}{
	OperationIssuesList: {
		"milestone": {}, "state": {}, "assignee": {}, "creator": {}, "mentioned": {},
		"labels": {}, "sort": {}, "direction": {}, "since": {}, "per_page": {}, "page": {},
	},
	OperationIssuesGet: {},
	OperationIssueCommentsList: {
		"since": {}, "per_page": {}, "page": {},
	},
}

// Route is a validated issues-read-v1 REST request.
type Route struct {
	Operation string
	Owner     string
	Repo      string
	Number    string
}

// FullRepo returns the canonical lowercase owner/name target.
func (r *Route) FullRepo() string {
	return strings.ToLower(r.Owner + "/" + r.Repo)
}

// MatchRoute matches only the versioned enclave issue-read REST surface.
func MatchRoute(path string, query url.Values) (*Route, error) {
	logRoute.Printf("Matching enclave route: path=%s", path)
	var route Route
	for _, candidate := range enclaveRoutes {
		match := restroute.Match(path, candidate.pattern)
		if match == nil {
			continue
		}
		route = Route{Operation: candidate.operation, Owner: match[1], Repo: match[2]}
		if len(match) > 3 {
			route.Number = match[3]
		}
		break
	}
	if route.Operation == "" {
		logRoute.Printf("No route pattern matched for path: %s", path)
		return nil, fmt.Errorf("unsupported enclave route")
	}
	fullRepo := route.Owner + "/" + route.Repo
	normalizedRepo, valid := NormalizeRepository(fullRepo)
	if !valid || normalizedRepo != fullRepo {
		logRoute.Printf("Repository normalization failed or mismatched: repo=%s, valid=%v", fullRepo, valid)
		return nil, fmt.Errorf("unsupported enclave route")
	}

	allowed := allowedQueryKeys[route.Operation]
	for key, values := range query {
		if _, ok := allowed[key]; !ok || len(values) != 1 {
			logRoute.Printf("Rejected unsupported query parameter: key=%s, operation=%s", key, route.Operation)
			return nil, fmt.Errorf("unsupported enclave query")
		}
	}
	logRoute.Printf("Route matched successfully: operation=%s, repo=%s", route.Operation, fullRepo)
	return &route, nil
}

package enclavegithub

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	issuesListPath = regexp.MustCompile(`^/repos/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/issues$`)
	issueGetPath   = regexp.MustCompile(`^/repos/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/issues/([1-9][0-9]*)$`)
	commentsPath   = regexp.MustCompile(`^/repos/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/issues/([1-9][0-9]*)/comments$`)
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
	var route Route
	switch {
	case issuesListPath.MatchString(path):
		match := issuesListPath.FindStringSubmatch(path)
		route = Route{Operation: OperationIssuesList, Owner: match[1], Repo: match[2]}
	case issueGetPath.MatchString(path):
		match := issueGetPath.FindStringSubmatch(path)
		route = Route{Operation: OperationIssuesGet, Owner: match[1], Repo: match[2], Number: match[3]}
	case commentsPath.MatchString(path):
		match := commentsPath.FindStringSubmatch(path)
		route = Route{Operation: OperationIssueCommentsList, Owner: match[1], Repo: match[2], Number: match[3]}
	default:
		return nil, fmt.Errorf("unsupported enclave route")
	}
	fullRepo := route.Owner + "/" + route.Repo
	normalizedRepo, valid := NormalizeRepository(fullRepo)
	if !valid || normalizedRepo != fullRepo {
		return nil, fmt.Errorf("unsupported enclave route")
	}

	allowed := allowedQueryKeys[route.Operation]
	for key, values := range query {
		if _, ok := allowed[key]; !ok || len(values) != 1 {
			return nil, fmt.Errorf("unsupported enclave query")
		}
	}
	return &route, nil
}

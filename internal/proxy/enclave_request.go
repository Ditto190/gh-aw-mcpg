package proxy

import (
	"net/http"
	"net/url"

	"github.com/github/gh-aw-mcpg/internal/enclavegithub"
	"github.com/github/gh-aw-mcpg/internal/logger"
)

var logEnclaveRequest = logger.ForFile()

// enclaveDenial identifies the stage that rejected an enclave-style request.
// Every stage produces the same client-visible denial; the distinct values only
// exist so each handler can emit its own log message.
type enclaveDenial int

const (
	// enclaveDenialNone means the request shape was accepted.
	enclaveDenialNone enclaveDenial = iota
	// enclaveDenialPath means the request path is not an enclave path.
	enclaveDenialPath
	// enclaveDenialRequestShape means the method, body, or query is invalid.
	enclaveDenialRequestShape
	// enclaveDenialRoute means no enclave route matched the request.
	enclaveDenialRoute
	// enclaveDenialTool means the matched route has no backing MCP tool.
	enclaveDenialTool
)

// enclaveRequestPlan is the parsed, route-matched shape of an enclave-style
// GitHub API request. It is produced by planEnclaveRequest so that the
// capability-verified enclave handler and the delegated-identity handler share
// one implementation of the security-critical parsing and dispatch shape.
type enclaveRequestPlan struct {
	// path is the enclave-relative request path. It is populated whenever the
	// path itself is valid, even if a later stage denied the request.
	path string
	// fullPath is path with the original raw query re-attached. It is only
	// populated for accepted requests.
	fullPath string
	// route is the matched enclave route, populated once route matching
	// succeeds.
	route *enclavegithub.Route
	// toolName and args are the backend MCP tool invocation for route.
	toolName string
	args     map[string]interface{}
	// denial records which stage rejected the request.
	denial enclaveDenial
}

// ok reports whether the request shape was accepted. Callers must still apply
// their own authorization before dispatching the plan.
func (p enclaveRequestPlan) ok() bool {
	return p.denial == enclaveDenialNone
}

// planEnclaveRequest applies the request-shape rules shared by every enclave
// admission point: enclave path extraction, GET-without-body enforcement, query
// parsing, route matching, tool/argument resolution, and full-path
// reconstruction.
//
// Authorization is intentionally left to the caller because it is the one part
// that legitimately differs: handleEnclaveRequest verifies capability claims
// (and additionally enforces the cross-repo public-visibility rule), while
// handleDelegatedRequest authorizes against the delegated-identity store.
func planEnclaveRequest(r *http.Request) enclaveRequestPlan {
	logEnclaveRequest.Printf("planEnclaveRequest: method=%s", r.Method)
	path, ok := enclavePath(r.URL.Path, r.URL.RawPath)
	if !ok {
		logEnclaveRequest.Print("planEnclaveRequest: denied, request path is not an enclave path")
		return enclaveRequestPlan{denial: enclaveDenialPath}
	}
	return planEnclaveRequestForPath(r, path)
}

// planEnclaveRequestForPath performs the planning stages after path extraction.
// The enclave handler calls it only after capability verification so route
// matching and its diagnostics cannot run for unauthenticated requests.
func planEnclaveRequestForPath(r *http.Request, path string) enclaveRequestPlan {
	plan := enclaveRequestPlan{path: path}

	if r.Method != http.MethodGet || hasEnclaveGETBody(r) {
		logEnclaveRequest.Printf("planEnclaveRequestForPath: denied, invalid request shape (method=%s, hasBody=%t)", r.Method, hasEnclaveGETBody(r))
		plan.denial = enclaveDenialRequestShape
		return plan
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		logEnclaveRequest.Print("planEnclaveRequestForPath: denied, failed to parse query string")
		plan.denial = enclaveDenialRequestShape
		return plan
	}

	route, err := enclavegithub.MatchEnclaveRoute(path, query)
	if err != nil {
		logEnclaveRequest.Print("planEnclaveRequestForPath: denied, no enclave route matched")
		plan.denial = enclaveDenialRoute
		return plan
	}
	plan.route = route

	toolName, args := enclaveToolAndArgs(route)
	if toolName == "" {
		logEnclaveRequest.Print("planEnclaveRequestForPath: denied, matched route has no backing MCP tool")
		plan.denial = enclaveDenialTool
		return plan
	}
	plan.toolName = toolName
	plan.args = args

	plan.fullPath = path
	if r.URL.RawQuery != "" {
		plan.fullPath += "?" + r.URL.RawQuery
	}
	logEnclaveRequest.Printf("planEnclaveRequestForPath: accepted, tool=%s", toolName)
	return plan
}

// routeAllowedBy reports whether the plan matched a route whose operation is
// permitted by claims. Plans denied before route matching have no route and are
// never allowed, so route is only dereferenced once matching succeeded.
func (p enclaveRequestPlan) routeAllowedBy(claims *enclavegithub.Claims) bool {
	return p.route != nil && claims.AllowsOperation(p.route.Operation)
}

package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/github/gh-aw-mcpg/internal/enclavegithub"
	"github.com/github/gh-aw-mcpg/internal/githubhttp"
	"github.com/github/gh-aw-mcpg/internal/httputil"
)

const maxEnclaveVisibilityResponseBytes = 1024 * 1024

type enclaveState struct {
	policy   *enclavegithub.Policy
	verifier *enclavegithub.Verifier

	visibilityMu        sync.RWMutex
	visibilityDecisions map[string]bool
}

func newEnclaveState(policy *enclavegithub.Policy, verifier *enclavegithub.Verifier) *enclaveState {
	return &enclaveState{
		policy:              policy,
		verifier:            verifier,
		visibilityDecisions: make(map[string]bool),
	}
}

type enclaveAgentIDContextKey struct{}

func withEnclaveAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, enclaveAgentIDContextKey{}, agentID)
}

func agentIDFromContext(ctx context.Context) string {
	agentID, _ := ctx.Value(enclaveAgentIDContextKey{}).(string)
	if agentID == "" {
		return proxyAgentID
	}
	return agentID
}

func (s *Server) enclaveRepositoryIsPublic(ctx context.Context, repo string) bool {
	s.enclave.visibilityMu.RLock()
	decision, cached := s.enclave.visibilityDecisions[repo]
	s.enclave.visibilityMu.RUnlock()
	if cached {
		return decision
	}

	resp, err := s.forwardEnclaveVisibilityLookup(ctx, repo)
	if err != nil {
		s.cacheEnclaveVisibilityDenial(repo)
		return false
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.cacheEnclaveVisibilityDenial(repo)
		return false
	}

	limited := io.LimitReader(resp.Body, maxEnclaveVisibilityResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil || len(body) > maxEnclaveVisibilityResponseBytes {
		s.cacheEnclaveVisibilityDenial(repo)
		return false
	}
	var visibility struct {
		Visibility string `json:"visibility"`
	}
	if err := json.Unmarshal(body, &visibility); err != nil || visibility.Visibility != "public" {
		s.cacheEnclaveVisibilityDenial(repo)
		return false
	}

	// Positive visibility is deliberately not cached because a repository can
	// become private during a workflow run.
	return true
}

func (s *Server) forwardEnclaveVisibilityLookup(ctx context.Context, repo string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.githubAPIURL+"/repos/"+repo, nil)
	if err != nil {
		return nil, err
	}
	githubhttp.ApplyGitHubAPIHeaders(req, "token "+s.githubToken)
	return s.httpClient.Do(req)
}

func (s *Server) cacheEnclaveVisibilityDenial(repo string) {
	s.enclave.visibilityMu.Lock()
	s.enclave.visibilityDecisions[repo] = false
	s.enclave.visibilityMu.Unlock()
}

func enclavePath(path string, rawPath string) (string, bool) {
	if rawPath != "" {
		return "", false
	}
	if path == ghHostPathPrefix {
		return "", false
	}
	if strings.HasPrefix(path, ghHostPathPrefix+"/") {
		return strings.TrimPrefix(path, ghHostPathPrefix), true
	}
	if strings.HasPrefix(path, ghHostPathPrefix) {
		return "", false
	}
	return path, true
}

func enclaveToolAndArgs(route *enclavegithub.Route) (string, map[string]interface{}) {
	switch route.Operation {
	case enclavegithub.OperationIssuesList:
		return "list_issues", repoArgs(route.Owner, route.Repo)
	case enclavegithub.OperationIssuesGet:
		return "issue_read", issueArgs(route.Owner, route.Repo, route.Number)
	case enclavegithub.OperationIssueCommentsList:
		return "issue_read", issueArgs(route.Owner, route.Repo, route.Number, "get_comments")
	default:
		return "", nil
	}
}

func writeEnclaveDenied(w http.ResponseWriter) {
	httputil.WriteErrorResponse(
		w,
		http.StatusForbidden,
		"enclave_access_denied",
		"enclave GitHub access denied",
	)
}

func hasEnclaveGETBody(r *http.Request) bool {
	return r.ContentLength != 0 || len(r.TransferEncoding) != 0
}

func (h *proxyHandler) handleEnclaveRequest(w http.ResponseWriter, r *http.Request) {
	path, ok := enclavePath(r.URL.Path, r.URL.RawPath)
	if !ok {
		writeEnclaveDenied(w)
		return
	}

	claims, err := h.server.enclave.verifier.VerifyAuthorization(r.Header.Get("Authorization"))
	if err != nil {
		writeEnclaveDenied(w)
		return
	}
	if r.Method != http.MethodGet || hasEnclaveGETBody(r) {
		writeEnclaveDenied(w)
		return
	}

	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeEnclaveDenied(w)
		return
	}
	route, err := enclavegithub.MatchRoute(path, query)
	if err != nil || !claims.AllowsOperation(route.Operation) {
		writeEnclaveDenied(w)
		return
	}

	targetRepo := route.FullRepo()
	if targetRepo != claims.Repo && !h.server.enclaveRepositoryIsPublic(r.Context(), targetRepo) {
		writeEnclaveDenied(w)
		return
	}

	toolName, args := enclaveToolAndArgs(route)
	if toolName == "" {
		writeEnclaveDenied(w)
		return
	}
	fullPath := path
	if r.URL.RawQuery != "" {
		fullPath += "?" + r.URL.RawQuery
	}
	ctx := withEnclaveAgentID(r.Context(), claims.AgentID())
	h.handleWithDIFC(w, r.WithContext(ctx), fullPath, toolName, args, nil)
}

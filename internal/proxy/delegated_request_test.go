package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/github/gh-aw-mcpg/internal/delegation"
	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDelegatedTestHandler builds a proxyHandler wired with a real delegation.Store
// and a stub guard that labels all resources public (no DIFC restrictions), so
// tests can focus on handleDelegatedRequest's own routing/authorization logic.
func newDelegatedTestHandler(t *testing.T, upstreamURL string, allowedRepos ...string) (*proxyHandler, *delegation.Store) {
	t.Helper()
	envelope := &delegation.Envelope{
		RunID:               "run-1",
		EnclaveBackend:      "backend",
		AllowedRepositories: allowedRepos,
		ToolPolicy:          delegation.ToolPolicyGitHubRepositoryReadV1,
		AllowedSchemaHashes: []string{"sha256:test"},
		MaxIdentityTTL:      time.Hour,
		ExpiresAt:           time.Now().Add(time.Hour),
	}
	store, err := delegation.NewStore(envelope, 1)
	require.NoError(t, err)

	g := &stubGuard{
		labelResourceResult: publicResource(),
		labelResourceOp:     difc.OperationRead,
	}
	server := newTestServerWithStub(t, upstreamURL, g, difc.EnforcementPropagate)
	server.githubToken = enclaveTestUpstreamToken
	server.delegation = &delegationState{
		store:     store,
		statePath: t.TempDir() + "/state.json",
	}
	return &proxyHandler{server: server}, store
}

// createDelegatedIdentity admits a fresh executor identity bound to repo and
// returns its opaque executor bearer token (unprefixed).
func createDelegatedIdentity(t *testing.T, store *delegation.Store, repo string) string {
	t.Helper()
	result, err := store.CreateOrConfirm(delegation.CreateOrConfirmRequest{
		RunID:          "run-1",
		EnclaveBackend: "backend",
		EnclaveEntryID: "entry-1",
		InvocationID:   "invocation-1",
		Repository:     repo,
		ToolPolicy:     delegation.ToolPolicyGitHubRepositoryReadV1,
		SchemaHash:     "sha256:test",
		RequestedTTL:   time.Minute,
		IdempotencyKey: "idempotency-key-1",
	})
	require.NoError(t, err)
	return result.ExecutorBearer
}

func delegatedRequest(h http.Handler, method, path, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)
	return recorder
}

func TestHandleDelegatedRequest_AuthorizedIssuesListSucceeds(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/octo/repo/issues", r.URL.Path)
		assert.Equal(t, "token "+enclaveTestUpstreamToken, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer upstream.Close()

	handler, store := newDelegatedTestHandler(t, upstream.URL, "octo/repo")
	bearer := createDelegatedIdentity(t, store, "octo/repo")

	rec := delegatedRequest(handler, http.MethodGet, "/repos/octo/repo/issues", bearer)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleDelegatedRequest_AuthorizedIssueGetSucceeds(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/octo/repo/issues/42", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"number":42}`))
	}))
	defer upstream.Close()

	handler, store := newDelegatedTestHandler(t, upstream.URL, "octo/repo")
	bearer := createDelegatedIdentity(t, store, "octo/repo")

	rec := delegatedRequest(handler, http.MethodGet, "/repos/octo/repo/issues/42", bearer)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleDelegatedRequest_AuthorizedCommentsListSucceedsWithQuery(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/octo/repo/issues/42/comments", r.URL.Path)
		assert.Equal(t, "100", r.URL.Query().Get("per_page"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer upstream.Close()

	handler, store := newDelegatedTestHandler(t, upstream.URL, "octo/repo")
	bearer := createDelegatedIdentity(t, store, "octo/repo")

	rec := delegatedRequest(handler, http.MethodGet, "/repos/octo/repo/issues/42/comments?per_page=100", bearer)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleDelegatedRequest_InvalidEnclavePathDenied(t *testing.T) {
	// enclavePath rejects requests that carry a RawPath distinct from Path
	// (unusual percent-encoding), which is the !ok branch.
	handler, _ := newDelegatedTestHandler(t, "http://unused.invalid", "octo/repo")
	req := httptest.NewRequest(http.MethodGet, "/repos/octo/repo/issues", nil)
	req.URL.RawPath = "/repos/octo/repo%2Fissues"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleDelegatedRequest_NonGETMethodDenied(t *testing.T) {
	handler, store := newDelegatedTestHandler(t, "http://unused.invalid", "octo/repo")
	bearer := createDelegatedIdentity(t, store, "octo/repo")

	rec := delegatedRequest(handler, http.MethodPost, "/repos/octo/repo/issues", bearer)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleDelegatedRequest_GETWithBodyDenied(t *testing.T) {
	handler, store := newDelegatedTestHandler(t, "http://unused.invalid", "octo/repo")
	bearer := createDelegatedIdentity(t, store, "octo/repo")

	req := httptest.NewRequest(http.MethodGet, "/repos/octo/repo/issues", nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.ContentLength = 5
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleDelegatedRequest_MalformedQueryDenied(t *testing.T) {
	handler, store := newDelegatedTestHandler(t, "http://unused.invalid", "octo/repo")
	bearer := createDelegatedIdentity(t, store, "octo/repo")

	req := httptest.NewRequest(http.MethodGet, "/repos/octo/repo/issues", nil)
	req.URL.RawQuery = "%zz"
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleDelegatedRequest_UnmatchedRouteDenied(t *testing.T) {
	handler, store := newDelegatedTestHandler(t, "http://unused.invalid", "octo/repo")
	bearer := createDelegatedIdentity(t, store, "octo/repo")

	rec := delegatedRequest(handler, http.MethodGet, "/repos/octo/repo/pulls", bearer)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleDelegatedRequest_UnsupportedQueryParamDenied(t *testing.T) {
	// issues.get (single-issue GET) allows no query keys at all, so any
	// query parameter makes MatchRoute fail even though the path matches.
	handler, store := newDelegatedTestHandler(t, "http://unused.invalid", "octo/repo")
	bearer := createDelegatedIdentity(t, store, "octo/repo")

	rec := delegatedRequest(handler, http.MethodGet, "/repos/octo/repo/issues/42?unsupported=1", bearer)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleDelegatedRequest_UnauthorizedExecutorBearerDenied(t *testing.T) {
	handler, _ := newDelegatedTestHandler(t, "http://unused.invalid", "octo/repo")

	rec := delegatedRequest(handler, http.MethodGet, "/repos/octo/repo/issues", "not-a-real-bearer")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleDelegatedRequest_MissingAuthorizationDenied(t *testing.T) {
	handler, _ := newDelegatedTestHandler(t, "http://unused.invalid", "octo/repo")

	rec := delegatedRequest(handler, http.MethodGet, "/repos/octo/repo/issues", "")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleDelegatedRequest_BearerAuthorizedForDifferentRepoDenied(t *testing.T) {
	// The identity is bound to octo/repo-a; requesting octo/repo-b must fail
	// even though the executor bearer itself is otherwise valid.
	handler, store := newDelegatedTestHandler(t, "http://unused.invalid", "octo/repo-a", "octo/repo-b")
	bearer := createDelegatedIdentity(t, store, "octo/repo-a")

	rec := delegatedRequest(handler, http.MethodGet, "/repos/octo/repo-b/issues", bearer)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleDelegatedRequest_PreservesQueryStringToUpstream(t *testing.T) {
	var gotRawQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer upstream.Close()

	handler, store := newDelegatedTestHandler(t, upstream.URL, "octo/repo")
	bearer := createDelegatedIdentity(t, store, "octo/repo")

	rec := delegatedRequest(handler, http.MethodGet, "/repos/octo/repo/issues?state=open&page=2", bearer)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "state=open&page=2", gotRawQuery)
}

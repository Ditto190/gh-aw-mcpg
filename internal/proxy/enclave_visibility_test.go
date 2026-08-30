package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnclaveRepositoryIsPublic exercises the branches of
// Server.enclaveRepositoryIsPublic: cached-denial (unexpired and expired),
// upstream transport error, non-200 HTTP status, oversized response body,
// malformed JSON, non-public visibility value, and the successful public
// path. Each case is a fresh Server/enclaveState so upstream call counts and
// cache state can be asserted independently.
func TestEnclaveRepositoryIsPublic(t *testing.T) {
	t.Run("cached denial not yet expired returns false without calling upstream", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		var upstreamCalls int
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			upstreamCalls++
			w.WriteHeader(http.StatusOK)
		}))
		defer upstream.Close()

		server := newTestServer(t, upstream.URL)
		server.githubToken = "test-token"
		server.enclave = newEnclaveState(nil, nil)

		currentTime := time.Unix(1_800_000_000, 0)
		server.enclave.now = func() time.Time { return currentTime }
		server.cacheEnclaveVisibilityDenial("org/repo")

		result := server.enclaveRepositoryIsPublic(context.Background(), "org/repo")

		assert.False(result, "should return false for an unexpired cached denial")
		require.Zero(upstreamCalls, "upstream should not be called when denial cache is fresh")
	})

	t.Run("cached denial expired clears entry and calls upstream", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		var upstreamCalls int
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			upstreamCalls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"visibility":"public"}`))
		}))
		defer upstream.Close()

		server := newTestServer(t, upstream.URL)
		server.githubToken = "test-token"
		server.enclave = newEnclaveState(nil, nil)

		currentTime := time.Unix(1_800_000_000, 0)
		server.enclave.now = func() time.Time { return currentTime }
		server.cacheEnclaveVisibilityDenial("org/repo")

		// Advance time past the TTL so the cached denial has expired.
		currentTime = currentTime.Add(enclaveVisibilityDeniedCacheTTL + time.Second)

		result := server.enclaveRepositoryIsPublic(context.Background(), "org/repo")

		assert.True(result, "should re-check upstream after cache expiry and find the repo public")
		require.Equal(1, upstreamCalls, "upstream should be called exactly once after cache expiry")

		server.enclave.visibilityMu.RLock()
		_, stillDenied := server.enclave.visibilityDecisions["org/repo"]
		server.enclave.visibilityMu.RUnlock()
		assert.False(stillDenied, "expired denial entry should be cleared, and success is not cached")
	})

	t.Run("upstream transport error caches denial and returns false", func(t *testing.T) {
		assert := assert.New(t)

		server := newTestServer(t, "http://127.0.0.1:0")
		server.githubToken = "test-token"
		server.enclave = newEnclaveState(nil, nil)
		// Port 0 lets request construction succeed but forces httpClient.Do to fail.

		result := server.enclaveRepositoryIsPublic(context.Background(), "org/repo")

		assert.False(result, "transport error should result in false")

		server.enclave.visibilityMu.RLock()
		_, denied := server.enclave.visibilityDecisions["org/repo"]
		server.enclave.visibilityMu.RUnlock()
		assert.True(denied, "transport error should populate the denial cache")
	})

	t.Run("non-200 HTTP status caches denial and returns false", func(t *testing.T) {
		assert := assert.New(t)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer upstream.Close()

		server := newTestServer(t, upstream.URL)
		server.githubToken = "test-token"
		server.enclave = newEnclaveState(nil, nil)

		result := server.enclaveRepositoryIsPublic(context.Background(), "org/repo")

		assert.False(result)
		server.enclave.visibilityMu.RLock()
		_, denied := server.enclave.visibilityDecisions["org/repo"]
		server.enclave.visibilityMu.RUnlock()
		assert.True(denied, "non-200 status should populate the denial cache")
	})

	t.Run("oversized response body caches denial and returns false", func(t *testing.T) {
		assert := assert.New(t)

		oversized := strings.Repeat("a", maxEnclaveVisibilityResponseBytes+2)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"visibility":"` + oversized + `"}`))
		}))
		defer upstream.Close()

		server := newTestServer(t, upstream.URL)
		server.githubToken = "test-token"
		server.enclave = newEnclaveState(nil, nil)

		result := server.enclaveRepositoryIsPublic(context.Background(), "org/repo")

		assert.False(result, "oversized body should be rejected")
		server.enclave.visibilityMu.RLock()
		_, denied := server.enclave.visibilityDecisions["org/repo"]
		server.enclave.visibilityMu.RUnlock()
		assert.True(denied)
	})

	t.Run("malformed JSON body caches denial and returns false", func(t *testing.T) {
		assert := assert.New(t)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{not-json`))
		}))
		defer upstream.Close()

		server := newTestServer(t, upstream.URL)
		server.githubToken = "test-token"
		server.enclave = newEnclaveState(nil, nil)

		result := server.enclaveRepositoryIsPublic(context.Background(), "org/repo")

		assert.False(result)
		server.enclave.visibilityMu.RLock()
		_, denied := server.enclave.visibilityDecisions["org/repo"]
		server.enclave.visibilityMu.RUnlock()
		assert.True(denied)
	})

	t.Run("non-public visibility value caches denial and returns false", func(t *testing.T) {
		assert := assert.New(t)

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"visibility":"private"}`))
		}))
		defer upstream.Close()

		server := newTestServer(t, upstream.URL)
		server.githubToken = "test-token"
		server.enclave = newEnclaveState(nil, nil)

		result := server.enclaveRepositoryIsPublic(context.Background(), "org/repo")

		assert.False(result)
		server.enclave.visibilityMu.RLock()
		_, denied := server.enclave.visibilityDecisions["org/repo"]
		server.enclave.visibilityMu.RUnlock()
		assert.True(denied)
	})

	t.Run("public visibility returns true and is not cached", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		var receivedAuth string
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"visibility":"public"}`))
		}))
		defer upstream.Close()

		server := newTestServer(t, upstream.URL)
		server.githubToken = "test-token"
		server.enclave = newEnclaveState(nil, nil)

		result := server.enclaveRepositoryIsPublic(context.Background(), "org/repo")

		assert.True(result)
		assert.Equal("token test-token", receivedAuth, "should forward the GitHub token to upstream")

		server.enclave.visibilityMu.RLock()
		_, denied := server.enclave.visibilityDecisions["org/repo"]
		server.enclave.visibilityMu.RUnlock()
		require.False(denied, "positive visibility results must not be cached")
	})
}

// TestForwardEnclaveVisibilityLookup verifies the HTTP request built by
// forwardEnclaveVisibilityLookup targets the expected path and carries the
// GitHub token, and that request construction/transport errors are surfaced.
func TestForwardEnclaveVisibilityLookup(t *testing.T) {
	t.Run("builds request against /repos/<repo> with token header", func(t *testing.T) {
		require := require.New(t)
		assert := assert.New(t)

		var gotPath, gotAuth string
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
		}))
		defer upstream.Close()

		server := newTestServer(t, upstream.URL)
		server.githubToken = "abc123"

		resp, err := server.forwardEnclaveVisibilityLookup(context.Background(), "octo/cat")
		require.NoError(err)
		defer resp.Body.Close()

		assert.Equal("/repos/octo/cat", gotPath)
		assert.Equal("token abc123", gotAuth)
		assert.Equal(http.StatusOK, resp.StatusCode)
	})

	t.Run("invalid request URL returns error", func(t *testing.T) {
		assert := assert.New(t)

		server := newTestServer(t, "http://127.0.0.1")
		server.githubAPIURL = "://not-a-valid-url"
		server.githubToken = "abc123"

		_, err := server.forwardEnclaveVisibilityLookup(context.Background(), "octo/cat")
		assert.Error(err, "malformed base URL should cause request construction to fail")
	})

	t.Run("transport error is returned to caller", func(t *testing.T) {
		assert := assert.New(t)

		server := newTestServer(t, "http://127.0.0.1:0")
		server.githubToken = "abc123"

		_, err := server.forwardEnclaveVisibilityLookup(context.Background(), "octo/cat")
		assert.Error(err)
	})
}

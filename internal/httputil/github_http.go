package httputil

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// GitHubUserAgent is the User-Agent header value sent on all GitHub API requests.
const GitHubUserAgent = "awmg/1.0"

// defaultGitHubHTTPClient applies a finite timeout so outbound GitHub API
// requests cannot hang indefinitely when no explicit context deadline is set.
var defaultGitHubHTTPClient = &http.Client{Timeout: 30 * time.Second}

// ApplyGitHubAPIHeaders sets the standard GitHub API request headers on req.
// authHeader should be the full Authorization header value (e.g. "token xyz" or
// "Bearer xyz"). When authHeader is empty no Authorization header is set, which
// is appropriate when the caller has already decided that no auth is available.
func ApplyGitHubAPIHeaders(req *http.Request, authHeader string) {
	logHTTP.Printf("Applying GitHub API headers: method=%s, path=%s, hasAuth=%v", req.Method, req.URL.Path, authHeader != "")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", GitHubUserAgent)
}

// DoGitHubGET sends an authenticated GET request to the GitHub API and returns
// the response. apiBaseURL is the API root (e.g. "https://api.github.com"),
// path is the request path (e.g. "/repos/owner/repo"), and authHeader is the
// full Authorization header value (e.g. "token xyz"). The caller is responsible
// for closing the response body. Request duration is bounded by whichever
// happens first: ctx cancellation/deadline or the helper client timeout.
func DoGitHubGET(ctx context.Context, apiBaseURL, path, authHeader string) (*http.Response, error) {
	logHTTP.Printf("GitHub GET: baseURL=%s, path=%s, hasAuth=%v", apiBaseURL, path, authHeader != "")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBaseURL+path, nil)
	if err != nil {
		logHTTP.Printf("Failed to create GitHub GET request: path=%s, err=%v", path, err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	ApplyGitHubAPIHeaders(req, authHeader)
	resp, err := defaultGitHubHTTPClient.Do(req)
	if err != nil {
		logHTTP.Printf("GitHub GET request failed: path=%s, err=%v", path, err)
		return nil, err
	}
	logHTTP.Printf("GitHub GET response: path=%s, status=%d", path, resp.StatusCode)
	return resp, nil
}

// ParseRateLimitResetHeader parses the Unix-timestamp value of the
// X-RateLimit-Reset HTTP header into a time.Time.
// Returns zero time when the header value is absent or malformed.
func ParseRateLimitResetHeader(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	unix, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		logHTTP.Printf("Failed to parse X-RateLimit-Reset header value=%q: %v", value, err)
		return time.Time{}
	}
	reset := time.Unix(unix, 0)
	logHTTP.Printf("Parsed X-RateLimit-Reset: resetAt=%s", reset.UTC().Format(time.RFC3339))
	return reset
}

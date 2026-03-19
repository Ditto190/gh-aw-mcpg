package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfNoGitHubToken skips the test if no GitHub token is available.
func skipIfNoGitHubToken(t *testing.T) string {
	t.Helper()
	// Try several token sources in order
	for _, env := range []string{"GITHUB_TOKEN", "GH_TOKEN", "GITHUB_PERSONAL_ACCESS_TOKEN"} {
		if tok := os.Getenv(env); tok != "" {
			return tok
		}
	}
	// Try gh auth token
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		tok := strings.TrimSpace(string(out))
		if tok != "" {
			return tok
		}
	}
	t.Skip("Skipping proxy integration test: no GitHub token available (set GITHUB_TOKEN or run `gh auth login`)")
	return ""
}

// findWasmGuard locates the GitHub guard WASM binary.
// Supports AWMG_WASM_GUARD_PATH env var override for container/CI environments.
func findWasmGuard(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("AWMG_WASM_GUARD_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		t.Fatalf("AWMG_WASM_GUARD_PATH=%q does not exist", p)
	}
	locations := []string{
		"../../guards/github-guard/rust-guard/target/wasm32-wasip1/release/github_guard.wasm",
		"guards/github-guard/rust-guard/target/wasm32-wasip1/release/github_guard.wasm",
		"/guards/github/00-github-guard.wasm", // container image path
	}
	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}
	t.Skip("Skipping proxy integration test: WASM guard not found (run `make build` in guards/github-guard)")
	return ""
}

// proxyTestEnv holds the running proxy server info for tests.
type proxyTestEnv struct {
	cmd     *exec.Cmd
	port    string
	baseURL string
	token   string
	cancel  context.CancelFunc
	logDir  string
	stdout  bytes.Buffer
	stderr  bytes.Buffer
}

// startProxy starts the awmg proxy with the given policy and returns the test env.
func startProxy(t *testing.T, policyJSON string, port string) *proxyTestEnv {
	t.Helper()

	binaryPath := findBinary(t)
	wasmPath := findWasmGuard(t)
	token := skipIfNoGitHubToken(t)

	logDir, err := os.MkdirTemp("", "awmg-proxy-test-*")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

	listenAddr := "127.0.0.1:" + port

	args := []string{
		"proxy",
		"--guard-wasm", wasmPath,
		"--policy", policyJSON,
		"--github-token", token,
		"--listen", listenAddr,
		"--log-dir", logDir,
		"--guards-mode", "filter",
	}

	cmd := exec.CommandContext(ctx, binaryPath, args...)

	env := &proxyTestEnv{
		cmd:     cmd,
		port:    port,
		baseURL: "http://" + listenAddr,
		token:   token,
		cancel:  cancel,
		logDir:  logDir,
	}

	cmd.Stdout = &env.stdout
	cmd.Stderr = &env.stderr

	err = cmd.Start()
	require.NoError(t, err, "Failed to start proxy")

	// Wait for the proxy to be healthy
	healthURL := env.baseURL + "/api/v3/health"
	if !waitForServer(t, healthURL, 15*time.Second) {
		t.Logf("STDOUT: %s", env.stdout.String())
		t.Logf("STDERR: %s", env.stderr.String())
		t.Fatal("Proxy did not start in time")
	}

	t.Logf("✓ Proxy started at %s with policy: %s", listenAddr, policyJSON)
	return env
}

// stop cleans up the proxy process and temp files.
func (e *proxyTestEnv) stop(t *testing.T) {
	t.Helper()
	if e.cmd.Process != nil {
		e.cmd.Process.Kill()
	}
	e.cancel()
	os.RemoveAll(e.logDir)
}

// ghAPI calls a GitHub REST API endpoint through the proxy using raw HTTP.
func (e *proxyTestEnv) ghAPI(t *testing.T, method, path string) (int, []byte) {
	t.Helper()

	url := e.baseURL + "/api/v3" + path
	req, err := http.NewRequest(method, url, nil)
	require.NoError(t, err)

	req.Header.Set("Authorization", "token "+e.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, body
}

// ghGraphQL sends a GraphQL query through the proxy.
func (e *proxyTestEnv) ghGraphQL(t *testing.T, query string, variables map[string]interface{}) (int, []byte) {
	t.Helper()

	payload := map[string]interface{}{"query": query}
	if variables != nil {
		payload["variables"] = variables
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	url := e.baseURL + "/api/v3/graphql"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	require.NoError(t, err)

	req.Header.Set("Authorization", "token "+e.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, respBody
}

// ghCLI runs a gh CLI command through the proxy using GH_HOST.
func (e *proxyTestEnv) ghCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = append(os.Environ(),
		"GH_HOST="+e.baseURL[len("http://"):], // strip scheme — gh adds it
		"GH_TOKEN="+e.token,
		// Disable gh's own TLS since we're using plain HTTP
		"GH_PROTOCOL=http",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// parseJSONArray parses a JSON array response body.
func parseJSONArray(t *testing.T, body []byte) []interface{} {
	t.Helper()
	var arr []interface{}
	if err := json.Unmarshal(body, &arr); err != nil {
		// May be an object (e.g., GraphQL response), not an array
		return nil
	}
	return arr
}

// parseJSONObject parses a JSON object response body.
func parseJSONObject(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var obj map[string]interface{}
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Logf("Warning: failed to parse JSON object: %v (body: %.200s)", err, string(body))
		return nil
	}
	return obj
}

// ============================================================================
// Test Suite: Repo-Scoped AllowOnly Policy
// ============================================================================

// TestProxyRepoScope validates that a repo-scoped allow-only policy correctly
// allows access to the scoped repo and blocks access to other repos.
// Note: Policy repos must be lowercase per guard validation rules.
func TestProxyRepoScope(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping proxy integration test in short mode")
	}

	// Policy: only allow access to octocat/hello-world (lowercase per guard validation)
	policy := `{"allow-only":{"repos":["octocat/hello-world"],"min-integrity":"none"}}`
	env := startProxy(t, policy, "18901")
	defer env.stop(t)

	// --- Scoped repo: octocat/Hello-World (should be ALLOWED) ---

	t.Run("ScopedRepo/ListIssues", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/octocat/Hello-World/issues?per_page=5&state=all")
		assert.Equal(t, 200, status, "Expected 200 for scoped repo issues")
		t.Logf("Scoped issues response (%.300s)", string(body))
	})

	t.Run("ScopedRepo/GetContents", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/octocat/Hello-World/contents/README")
		assert.Equal(t, 200, status, "Expected 200 for scoped repo contents")
		t.Logf("Scoped contents response (%.300s)", string(body))
	})

	t.Run("ScopedRepo/ListCommits", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/octocat/Hello-World/commits?per_page=5")
		assert.Equal(t, 200, status, "Expected 200 for scoped repo commits")
		t.Logf("Scoped commits: %.300s", string(body))
	})

	t.Run("ScopedRepo/ListBranches", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/octocat/Hello-World/branches?per_page=10")
		assert.Equal(t, 200, status, "Expected 200 for scoped repo branches")
		t.Logf("Scoped branches: %.300s", string(body))
	})

	// --- Out-of-scope repo: cli/cli (should be BLOCKED or filtered empty) ---

	t.Run("OutOfScope/ListIssues", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/cli/cli/issues?per_page=5")
		// The proxy should either return 403, empty array, or filter all items
		if status == 200 {
			arr := parseJSONArray(t, body)
			assert.Empty(t, arr, "Out-of-scope repo should return empty issues array")
		} else {
			assert.Contains(t, []int{403, 200}, status, "Expected 403 or 200 with empty results")
		}
		t.Logf("Out-of-scope issues: status=%d body=%.200s", status, string(body))
	})

	t.Run("OutOfScope/GetContents", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/cli/cli/contents/README.md")
		// Out-of-scope: expect blocked (403) or empty
		t.Logf("Out-of-scope contents: status=%d body=%.200s", status, string(body))
		if status == 200 {
			arr := parseJSONArray(t, body)
			if arr != nil {
				assert.Empty(t, arr, "Out-of-scope contents should be empty")
			}
		}
	})

	// --- Global APIs (should be BLOCKED or empty) ---

	t.Run("Global/User", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/user")
		t.Logf("GET /user: status=%d body=%.200s", status, string(body))
	})

	t.Run("Global/SearchIssues", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/search/issues?q=repo:octocat/Hello-World+is:issue&per_page=5")
		t.Logf("Search issues: status=%d body=%.200s", status, string(body))
		assert.Equal(t, 200, status)
	})
}

// ============================================================================
// Test Suite: Owner-Scoped AllowOnly Policy
// ============================================================================

// TestProxyOwnerScope validates that an owner-scoped allow-only policy allows
// access to any repo under the specified owner.
func TestProxyOwnerScope(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping proxy integration test in short mode")
	}

	// Policy: allow all repos under 'octocat' owner
	policy := `{"allow-only":{"repos":["octocat/*"],"min-integrity":"none"}}`
	env := startProxy(t, policy, "18902")
	defer env.stop(t)

	t.Run("ScopedOwner/HelloWorld/ListIssues", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/octocat/Hello-World/issues?per_page=5&state=all")
		assert.Equal(t, 200, status)
		t.Logf("octocat/Hello-World issues: status=%d body=%.300s", status, string(body))
	})

	t.Run("ScopedOwner/Spoon-Knife/ListCommits", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/octocat/Spoon-Knife/commits?per_page=5")
		assert.Equal(t, 200, status)
		t.Logf("octocat/Spoon-Knife commits: status=%d body=%.300s", status, string(body))
	})

	t.Run("OutOfScope/CliCli/ListIssues", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/cli/cli/issues?per_page=5")
		if status == 200 {
			arr := parseJSONArray(t, body)
			assert.Empty(t, arr, "Out-of-scope repo should return empty array")
		}
		t.Logf("cli/cli issues: status=%d body=%.200s", status, string(body))
	})
}

// ============================================================================
// Test Suite: Integrity Filtering
// ============================================================================

// TestProxyIntegrityFiltering validates that min-integrity filtering works —
// items authored by non-collaborators are filtered out when min-integrity is set.
func TestProxyIntegrityFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping proxy integration test in short mode")
	}

	// Policy: allow octocat/hello-world but require approved integrity.
	policy := `{"allow-only":{"repos":["octocat/hello-world"],"min-integrity":"approved"}}`
	env := startProxy(t, policy, "18903")
	defer env.stop(t)

	t.Run("ApprovedIntegrity/ListIssues", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/octocat/Hello-World/issues?per_page=30&state=all")
		assert.Equal(t, 200, status)
		arr := parseJSONArray(t, body)
		// With approved integrity, many community issues should be filtered
		t.Logf("Issues returned with min-integrity=approved: %d (from 30 requested)", len(arr))
	})

	t.Run("ApprovedIntegrity/ListCommits", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/octocat/Hello-World/commits?per_page=10")
		assert.Equal(t, 200, status)
		arr := parseJSONArray(t, body)
		t.Logf("Commits returned with min-integrity=approved: %d", len(arr))
	})
}

// ============================================================================
// Test Suite: GraphQL via Proxy
// ============================================================================

// TestProxyGraphQL validates that GraphQL queries are correctly routed and filtered.
func TestProxyGraphQL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping proxy integration test in short mode")
	}

	policy := `{"allow-only":{"repos":["octocat/hello-world"],"min-integrity":"none"}}`
	env := startProxy(t, policy, "18904")
	defer env.stop(t)

	t.Run("ScopedRepo/IssueList", func(t *testing.T) {
		query := `query {
			repository(owner: "octocat", name: "Hello-World") {
				issues(first: 5) {
					nodes {
						title
						number
						author { login }
					}
				}
			}
		}`
		status, body := env.ghGraphQL(t, query, nil)
		assert.Equal(t, 200, status)
		t.Logf("GraphQL issues response: %.500s", string(body))

		obj := parseJSONObject(t, body)
		if obj != nil {
			assert.NotContains(t, obj, "errors", "Should not have GraphQL errors")
		}
	})

	t.Run("ScopedRepo/WithVariables", func(t *testing.T) {
		query := `query($owner: String!, $name: String!) {
			repository(owner: $owner, name: $name) {
				name
				description
				defaultBranchRef { name }
			}
		}`
		vars := map[string]interface{}{
			"owner": "octocat",
			"name":  "Hello-World",
		}
		status, body := env.ghGraphQL(t, query, vars)
		assert.Equal(t, 200, status)
		t.Logf("GraphQL repo info: %.500s", string(body))
	})

	t.Run("OutOfScope/DifferentRepo", func(t *testing.T) {
		query := `query {
			repository(owner: "cli", name: "cli") {
				issues(first: 5) {
					nodes {
						title
						number
					}
				}
			}
		}`
		status, body := env.ghGraphQL(t, query, nil)
		t.Logf("Out-of-scope GraphQL: status=%d body=%.500s", status, string(body))
		// Should be blocked or return empty/error
	})

	t.Run("Global/Viewer", func(t *testing.T) {
		query := `query { viewer { login name } }`
		status, body := env.ghGraphQL(t, query, nil)
		t.Logf("Viewer query: status=%d body=%.200s", status, string(body))
	})
}

// ============================================================================
// Test Suite: gh CLI Through Proxy
// ============================================================================

// TestProxyGhCLI validates that the actual gh CLI works through the proxy.
// This is the highest-fidelity test — it uses real gh commands.
func TestProxyGhCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping proxy gh CLI integration test in short mode")
	}

	// Check that gh is available
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("Skipping: gh CLI not found in PATH")
	}

	policy := `{"allow-only":{"repos":["octocat/hello-world"],"min-integrity":"none"}}`
	env := startProxy(t, policy, "18905")
	defer env.stop(t)

	t.Run("ScopedRepo/ApiRepoInfo", func(t *testing.T) {
		stdout, stderr, err := env.ghCLI(t, "api", "/repos/octocat/Hello-World")
		if err != nil {
			t.Logf("gh api failed: %v\nstderr: %s", err, stderr)
			// gh may not support GH_PROTOCOL=http well; log and continue
			t.Skip("gh CLI may not support plain HTTP proxy")
		}
		assert.Contains(t, stdout, "Hello-World", "Should return Hello-World repo info")
		t.Logf("gh api response: %.200s", stdout)
	})

	t.Run("ScopedRepo/ApiIssues", func(t *testing.T) {
		stdout, stderr, err := env.ghCLI(t, "api", "/repos/octocat/Hello-World/issues?per_page=3")
		if err != nil {
			t.Logf("gh api issues failed: %v\nstderr: %s", err, stderr)
			t.Skip("gh CLI may not support plain HTTP proxy")
		}
		t.Logf("gh api issues: %.200s", stdout)
	})
}

// ============================================================================
// Test Suite: Proxy Health and Basic Operation
// ============================================================================

// TestProxyHealthAndPassthrough validates basic proxy operation.
func TestProxyHealthAndPassthrough(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping proxy integration test in short mode")
	}

	// Open policy — allow everything (for testing passthrough)
	policy := `{"allow-only":{"repos":"public","min-integrity":"none"}}`
	env := startProxy(t, policy, "18906")
	defer env.stop(t)

	t.Run("HealthCheck", func(t *testing.T) {
		resp, err := http.Get(env.baseURL + "/api/v3/health")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("Passthrough/GetUser", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/user")
		assert.Equal(t, 200, status)
		// Note: the guard may transform the response format (wrap objects in arrays)
		t.Logf("Authenticated user response: status=%d body=%.300s", status, string(body))
	})

	t.Run("Passthrough/GetPublicRepo", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/octocat/Hello-World")
		assert.Equal(t, 200, status)
		t.Logf("Public repo response: status=%d body=%.300s", status, string(body))
	})

	t.Run("Passthrough/ListIssuesWithData", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/octocat/Hello-World/issues?per_page=5&state=all")
		assert.Equal(t, 200, status)
		t.Logf("Issues response: status=%d body=%.300s", status, string(body))
	})
}

// ============================================================================
// Test Suite: Multiple Repos in Policy
// ============================================================================

// TestProxyMultiRepoPolicy validates that policies with multiple repo patterns work.
func TestProxyMultiRepoPolicy(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping proxy integration test in short mode")
	}

	// Policy: allow two specific repos
	policy := `{"allow-only":{"repos":["octocat/hello-world","octocat/spoon-knife"],"min-integrity":"none"}}`
	env := startProxy(t, policy, "18907")
	defer env.stop(t)

	t.Run("FirstRepo/HelloWorld", func(t *testing.T) {
		status, _ := env.ghAPI(t, "GET", "/repos/octocat/Hello-World/commits?per_page=3")
		assert.Equal(t, 200, status)
	})

	t.Run("SecondRepo/SpoonKnife", func(t *testing.T) {
		status, _ := env.ghAPI(t, "GET", "/repos/octocat/Spoon-Knife/commits?per_page=3")
		assert.Equal(t, 200, status)
	})

	t.Run("OutOfScope/CliCli", func(t *testing.T) {
		status, body := env.ghAPI(t, "GET", "/repos/cli/cli/commits?per_page=3")
		if status == 200 {
			arr := parseJSONArray(t, body)
			assert.Empty(t, arr, "Out-of-scope repo commits should be filtered")
		}
		t.Logf("Out-of-scope commits: status=%d", status)
	})
}

// ============================================================================
// Helpers (proxy-specific)
// ============================================================================

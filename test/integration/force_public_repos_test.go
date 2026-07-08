package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestForcePublicRepos_PublicRepo_OverridesAllowOnly verifies that when the
// workflow repository is public, the gateway overrides the allow-only policy to
// repos="public" and logs the FORCED REPOS=PUBLIC warning.
func TestForcePublicRepos_PublicRepo_OverridesAllowOnly(t *testing.T) {
	binary := binaryPath(t)
	port := getFreePort(t)
	logDir := t.TempDir()

	backend := startMockBackend(t)
	defer backend.Close()

	mockAPI := startMockGitHubAPI(t, "public", false)
	defer mockAPI.Close()

	config := fmt.Sprintf(`{
		"mcpServers": {
			"github": {
				"type": "http",
				"url": "%s",
				"guard-policies": {
					"allow-only": {
						"repos": "all",
						"min-integrity": "none"
					}
				}
			}
		},
		"gateway": {
			"port": %d,
			"domain": "localhost",
			"agentId": "test-key"
		}
	}`, backend.URL, port)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--config-stdin", "--log-dir", logDir)
	cmd.Stdin = strings.NewReader(config)
	filteredEnv := filterEnv(os.Environ(),
		"GITHUB_REPOSITORY", "GITHUB_TOKEN", "GITHUB_MCP_SERVER_TOKEN",
		"GITHUB_PERSONAL_ACCESS_TOKEN", "GH_TOKEN", "GITHUB_API_URL",
		"MCP_GATEWAY_FORCE_PUBLIC_REPOS",
	)
	filteredEnv = append(filteredEnv,
		"GITHUB_REPOSITORY=test-owner/test-repo",
		"GITHUB_TOKEN=mock-token-for-testing",
		"MCP_GATEWAY_FORCE_PUBLIC_REPOS=true",
		fmt.Sprintf("GITHUB_API_URL=%s", mockAPI.URL),
		"MCP_GATEWAY_WASM_GUARDS_DIR=",
	)
	cmd.Env = filteredEnv

	var stderr syncBuffer
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &stderr

	err := cmd.Start()
	require.NoError(t, err, "Failed to start gateway")

	ok := waitForStderr(&stderr, "Starting MCPG", 15*time.Second)
	require.Truef(t, ok, "timeout waiting for startup; stderr:\n%s", stderr.String())

	cmd.Process.Kill()
	cmd.Wait()

	logContent := readUnifiedLog(logDir)
	assert.Contains(t, logContent, "FORCED REPOS=PUBLIC",
		"Log should contain forced repos=public warning for a public repo")
	assert.Contains(t, logContent, "test-owner/test-repo",
		"Log should reference the repository name")
	t.Log("✓ Force-public-repos override triggered for public repo")
}

// TestForcePublicRepos_PrivateRepo_NoOverride verifies that when the workflow
// repository is private, the allow-only policy is left unchanged and no
// FORCED REPOS=PUBLIC warning is emitted.
func TestForcePublicRepos_PrivateRepo_NoOverride(t *testing.T) {
	binary := binaryPath(t)
	port := getFreePort(t)
	logDir := t.TempDir()

	backend := startMockBackend(t)
	defer backend.Close()

	mockAPI := startMockGitHubAPI(t, "private", true)
	defer mockAPI.Close()

	config := fmt.Sprintf(`{
		"mcpServers": {
			"github": {
				"type": "http",
				"url": "%s",
				"guard-policies": {
					"allow-only": {
						"repos": "all",
						"min-integrity": "none"
					}
				}
			}
		},
		"gateway": {
			"port": %d,
			"domain": "localhost",
			"agentId": "test-key"
		}
	}`, backend.URL, port)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--config-stdin", "--log-dir", logDir)
	cmd.Stdin = strings.NewReader(config)
	filteredEnv := filterEnv(os.Environ(),
		"GITHUB_REPOSITORY", "GITHUB_TOKEN", "GITHUB_MCP_SERVER_TOKEN",
		"GITHUB_PERSONAL_ACCESS_TOKEN", "GH_TOKEN", "GITHUB_API_URL",
	)
	filteredEnv = append(filteredEnv,
		"GITHUB_REPOSITORY=test-owner/private-repo",
		"GITHUB_TOKEN=mock-token-for-testing",
		fmt.Sprintf("GITHUB_API_URL=%s", mockAPI.URL),
		"MCP_GATEWAY_WASM_GUARDS_DIR=",
	)
	cmd.Env = filteredEnv

	var stderr syncBuffer
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &stderr

	err := cmd.Start()
	require.NoError(t, err, "Failed to start gateway")

	ok := waitForStderr(&stderr, "Starting MCPG", 15*time.Second)
	require.Truef(t, ok, "timeout waiting for startup; stderr:\n%s", stderr.String())

	cmd.Process.Kill()
	cmd.Wait()

	logContent := readUnifiedLog(logDir)
	assert.NotContains(t, logContent, "FORCED REPOS=PUBLIC",
		"Should NOT log forced repos=public for a private repo")
	t.Log("✓ No force-public-repos override for private repo")
}

// TestForcePublicRepos_ConfigOptOut_NoOverride verifies that when
// gateway.forcePublicRepos=false is set in config, no override is applied
// even when the repo is public.
func TestForcePublicRepos_ConfigOptOut_NoOverride(t *testing.T) {
	binary := binaryPath(t)
	port := getFreePort(t)
	logDir := t.TempDir()

	backend := startMockBackend(t)
	defer backend.Close()

	mockAPI := startMockGitHubAPI(t, "public", false)
	defer mockAPI.Close()

	config := fmt.Sprintf(`{
		"mcpServers": {
			"github": {
				"type": "http",
				"url": "%s",
				"guard-policies": {
					"allow-only": {
						"repos": "all",
						"min-integrity": "none"
					}
				}
			}
		},
		"gateway": {
			"port": %d,
			"domain": "localhost",
			"agentId": "test-key",
			"forcePublicRepos": false
		}
	}`, backend.URL, port)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--config-stdin", "--log-dir", logDir)
	cmd.Stdin = strings.NewReader(config)
	filteredEnv := filterEnv(os.Environ(),
		"GITHUB_REPOSITORY", "GITHUB_TOKEN", "GITHUB_MCP_SERVER_TOKEN",
		"GITHUB_PERSONAL_ACCESS_TOKEN", "GH_TOKEN", "GITHUB_API_URL",
	)
	filteredEnv = append(filteredEnv,
		"GITHUB_REPOSITORY=test-owner/test-repo",
		"GITHUB_TOKEN=mock-token-for-testing",
		fmt.Sprintf("GITHUB_API_URL=%s", mockAPI.URL),
		"MCP_GATEWAY_WASM_GUARDS_DIR=",
	)
	cmd.Env = filteredEnv

	var stderr syncBuffer
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &stderr

	err := cmd.Start()
	require.NoError(t, err, "Failed to start gateway")

	ok := waitForStderr(&stderr, "Starting MCPG", 15*time.Second)
	require.Truef(t, ok, "timeout waiting for startup; stderr:\n%s", stderr.String())

	cmd.Process.Kill()
	cmd.Wait()

	logContent := readUnifiedLog(logDir)
	assert.NotContains(t, logContent, "FORCED REPOS=PUBLIC",
		"Should NOT log forced repos=public when forcePublicRepos=false in config")
	t.Log("✓ No force-public-repos override when disabled in config")
}

// TestForcePublicRepos_NoGitHubRepository_NoOverride verifies that when
// GITHUB_REPOSITORY is not set, no override is applied and the gateway starts
// cleanly.
func TestForcePublicRepos_NoGitHubRepository_NoOverride(t *testing.T) {
	binary := binaryPath(t)
	port := getFreePort(t)
	logDir := t.TempDir()

	backend := startMockBackend(t)
	defer backend.Close()

	config := fmt.Sprintf(`{
		"mcpServers": {
			"github": {
				"type": "http",
				"url": "%s",
				"guard-policies": {
					"allow-only": {
						"repos": "all",
						"min-integrity": "none"
					}
				}
			}
		},
		"gateway": {
			"port": %d,
			"domain": "localhost",
			"agentId": "test-key"
		}
	}`, backend.URL, port)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, "--config-stdin", "--log-dir", logDir)
	cmd.Stdin = strings.NewReader(config)

	// Explicitly remove GITHUB_REPOSITORY
	filteredEnv := filterEnv(os.Environ(), "GITHUB_REPOSITORY")
	filteredEnv = append(filteredEnv, "MCP_GATEWAY_WASM_GUARDS_DIR=")
	cmd.Env = filteredEnv

	var stderr syncBuffer
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &stderr

	err := cmd.Start()
	require.NoError(t, err, "Failed to start gateway")

	ok := waitForStderr(&stderr, "Starting MCPG", 15*time.Second)
	require.Truef(t, ok, "timeout waiting for startup; stderr:\n%s", stderr.String())

	cmd.Process.Kill()
	cmd.Wait()

	logContent := readUnifiedLog(logDir)
	assert.NotContains(t, logContent, "FORCED REPOS=PUBLIC",
		"Should NOT log forced repos=public without GITHUB_REPOSITORY")
	t.Log("✓ No force-public-repos override without GITHUB_REPOSITORY — clean startup")
}

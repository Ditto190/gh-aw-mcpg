package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/github/gh-aw-mcpg/internal/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runWithStdin temporarily replaces os.Stdin with a pipe fed with the given
// content, invokes run() and restores os.Stdin before returning. This mirrors
// the pattern used throughout internal/config's test suite for exercising
// LoadFromStdin (which reads directly from os.Stdin).
func runWithStdin(t *testing.T, content string) error {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	go func() {
		_, _ = w.Write([]byte(content))
		_ = w.Close()
	}()

	return run(rootCmd, nil)
}

// resetRunFlagsForTest saves the current values of all package-level flag
// variables consumed by run() and returns a restore function. This allows
// tests to freely mutate global flag state (set by Cobra during normal CLI
// parsing) without leaking changes into other tests.
func resetRunFlagsForTest(t *testing.T) {
	t.Helper()

	origConfigFile := configFile
	origConfigStdin := configStdin
	origListenAddr := listenAddr
	origRoutedMode := routedMode
	origUnifiedMode := unifiedMode
	origEnvFile := envFile
	origLogDir := logDir
	origPayloadDir := payloadDir
	origPayloadPathPrefix := payloadPathPrefix
	origPayloadSizeThreshold := payloadSizeThreshold
	origURLDomainAudit := urlDomainAudit
	origWasmCacheDir := wasmCacheDir
	origShutdownTimeout := shutdownTimeout
	origTLSCertPath := tlsCertPath
	origTLSKeyPath := tlsKeyPath
	origTLSCAPath := tlsCAPath
	origHMACSecret := hmacSecret
	origDifcMode := difcMode
	origDifcSinkServerIDs := difcSinkServerIDs
	origSequentialLaunch := sequentialLaunch

	t.Cleanup(func() {
		configFile = origConfigFile
		configStdin = origConfigStdin
		listenAddr = origListenAddr
		routedMode = origRoutedMode
		unifiedMode = origUnifiedMode
		envFile = origEnvFile
		logDir = origLogDir
		payloadDir = origPayloadDir
		payloadPathPrefix = origPayloadPathPrefix
		payloadSizeThreshold = origPayloadSizeThreshold
		urlDomainAudit = origURLDomainAudit
		wasmCacheDir = origWasmCacheDir
		shutdownTimeout = origShutdownTimeout
		tlsCertPath = origTLSCertPath
		tlsKeyPath = origTLSKeyPath
		tlsCAPath = origTLSCAPath
		hmacSecret = origHMACSecret
		difcMode = origDifcMode
		difcSinkServerIDs = origDifcSinkServerIDs
		sequentialLaunch = origSequentialLaunch
	})
}

// writeMinimalConfigFile writes a minimal valid TOML config (no backend
// servers) to a temp file and returns its path.
func writeMinimalConfigFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[gateway]
port = 3000
agent_id = "test-agent-id"

[servers.testserver]
type = "http"
url = "http://127.0.0.1:1"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestRun_GracefulShutdownViaContextCancellation exercises the full run()
// startup path end-to-end: loading config from file, creating the WASM
// compilation cache, building the unified server, starting the HTTP listener,
// and then observing a clean shutdown when the context is cancelled (as
// happens on SIGINT/SIGTERM in production). This covers run()'s happy path
// and its background shutdown goroutine, which previously had 0% coverage.
func TestRun_GracefulShutdownViaContextCancellation(t *testing.T) {
	resetRunFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	envFile = ""
	logDir = t.TempDir()
	payloadDir = t.TempDir()
	payloadPathPrefix = ""
	payloadSizeThreshold = 0
	urlDomainAudit = false
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	tlsCertPath = ""
	tlsKeyPath = ""
	tlsCAPath = ""
	hmacSecret = ""
	difcMode = "strict"
	difcSinkServerIDs = ""
	sequentialLaunch = false

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(rootCmd, nil)
	}()

	// Give the server a brief moment to finish starting up (config load,
	// wasm cache setup, unified server construction, listener bind) before
	// requesting shutdown.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should shut down gracefully without error when context is cancelled")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

// TestRun_InvalidConfigFile verifies that run() surfaces a wrapped error when
// the configured file cannot be loaded, without starting any server.
func TestRun_InvalidConfigFile(t *testing.T) {
	resetRunFlagsForTest(t)

	configFile = filepath.Join(t.TempDir(), "does-not-exist.toml")
	configStdin = false
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	envFile = ""
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	rootCmd.SetContext(context.Background())
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	err := run(rootCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

func TestRun_RequiresConfiguredAgentID(t *testing.T) {
	resetRunFlagsForTest(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[gateway]
port = 3000

[servers.testserver]
type = "http"
url = "http://127.0.0.1:1"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	configFile = path
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	rootCmd.SetContext(context.Background())
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	err := run(rootCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gateway.agent_id or gateway.agent_ids must be configured; exactly one selection is required")
}

// TestRun_InvalidEnvFile verifies that run() returns a wrapped error when the
// configured --env-file path does not exist, before any config loading or
// server startup occurs.
func TestRun_InvalidEnvFile(t *testing.T) {
	resetRunFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = filepath.Join(t.TempDir(), "does-not-exist.env")
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	rootCmd.SetContext(context.Background())
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	err := run(rootCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load .env file")
}

// TestRun_InvalidGuardsMode verifies that run() rejects an unrecognized
// --guards-mode value early, via applyLaunchAndGuardsOverrides, before
// attempting to start the server.
func TestRun_InvalidGuardsMode(t *testing.T) {
	resetRunFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "not-a-real-mode"

	// Simulate the flag having been explicitly set on the command line so that
	// applyLaunchAndGuardsOverrides validates it.
	cmd := rootCmd
	require.NoError(t, cmd.Flags().Set("guards-mode", "not-a-real-mode"))
	t.Cleanup(func() {
		_ = cmd.Flags().Set("guards-mode", "strict")
	})

	rootCmd.SetContext(context.Background())
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	err := run(rootCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --guards-mode flag")
}

// TestRun_PluralAgentIDsStartsSuccessfully verifies that run() starts normally
// when only gateway.agentIds (plural) is configured: every configured
// identifier is accepted as a valid Authorization credential (see
// config.Config.GetAgentIDs and authMiddleware's matchesAnyKey), enabling
// concurrent primary/enclave sessions that each authenticate independently.
func TestRun_PluralAgentIDsStartsSuccessfully(t *testing.T) {
	resetRunFlagsForTest(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[gateway]
port = 3000
agent_ids = ["primary-agent", "enclave-agent"]

[gateway.agent_policies.primary-agent]
servers = ["testserver"]

[gateway.agent_policies.enclave-agent]
servers = ["testserver"]

[servers.testserver]
type = "http"
url = "http://127.0.0.1:1"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	configFile = path
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(rootCmd, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should start successfully with only gateway.agentIds configured")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

// resetGuardPolicyFlagsForTest saves/restores the guard-policy-related flag
// variables so tests can set them without leaking state.
func resetGuardPolicyFlagsForTest(t *testing.T) {
	t.Helper()
	origGuardPolicyJSON := guardPolicyJSON
	origAllowOnlyPublic := allowOnlyPublic
	origAllowOnlyOwner := allowOnlyOwner
	origAllowOnlyRepo := allowOnlyRepo
	origAllowOnlyMinInt := allowOnlyMinInt
	t.Cleanup(func() {
		guardPolicyJSON = origGuardPolicyJSON
		allowOnlyPublic = origAllowOnlyPublic
		allowOnlyOwner = origAllowOnlyOwner
		allowOnlyRepo = origAllowOnlyRepo
		allowOnlyMinInt = origAllowOnlyMinInt
		_ = rootCmd.Flags().Set("guard-policy-json", "")
		_ = rootCmd.Flags().Set("allowonly-scope-public", "false")
		_ = rootCmd.Flags().Set("allowonly-scope-owner", "")
		_ = rootCmd.Flags().Set("allowonly-scope-repo", "")
		_ = rootCmd.Flags().Set("allowonly-min-integrity", "")
	})
}

// TestRun_ConfigFromStdin verifies that run() reads and loads configuration
// from stdin (rather than a file) when --config-stdin is set, exercising the
// configStdin branch of both the log-source selection and the config.LoadFromStdin
// call, which previously had 0% coverage.
func TestRun_ConfigFromStdin(t *testing.T) {
	resetRunFlagsForTest(t)

	configStdin = true
	configFile = ""
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	payloadDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	stdinJSON := `{
		"mcpServers": {
			"testserver": {
				"type": "http",
				"url": "http://127.0.0.1:1"
			}
		},
		"gateway": {
			"port": 3000,
			"domain": "localhost",
			"agentId": "test-agent-id"
		}
	}`

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- runWithStdin(t, stdinJSON)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should start successfully reading config from stdin")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

// TestRun_InvalidStdinConfig verifies that run() surfaces a wrapped error when
// --config-stdin is set but the piped JSON is invalid, exercising the stdin
// error-handling branch of config loading.
func TestRun_InvalidStdinConfig(t *testing.T) {
	resetRunFlagsForTest(t)

	configStdin = true
	configFile = ""
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	rootCmd.SetContext(context.Background())
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	err := runWithStdin(t, "not valid json{{{")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

// TestRun_MultipleServersLoggedWithNames verifies that run() logs server names
// when more than one MCP server is configured (the serverNames non-empty
// branch), and starts successfully.
func TestRun_MultipleServersLoggedWithNames(t *testing.T) {
	resetRunFlagsForTest(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[gateway]
port = 3000
agent_id = "test-agent-id"

[servers.serverone]
type = "http"
url = "http://127.0.0.1:1"

[servers.servertwo]
type = "http"
url = "http://127.0.0.1:2"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	configFile = path
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(rootCmd, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should start successfully with multiple servers configured")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

// TestRun_ValidateEnvFailsWithoutRequiredEnvVars verifies that run() returns a
// wrapped environment-validation error when --validate-env is set and the
// required MCP_GATEWAY_* environment variables are absent, without starting
// any server. This exercises the validateEnv branch and its error-return path.
func TestRun_ValidateEnvFailsWithoutRequiredEnvVars(t *testing.T) {
	resetRunFlagsForTest(t)

	origValidateEnv := validateEnv
	t.Cleanup(func() { validateEnv = origValidateEnv })

	for _, name := range []string{"MCP_GATEWAY_PORT", "MCP_GATEWAY_DOMAIN", "MCP_GATEWAY_AGENT_ID"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"
	validateEnv = true

	rootCmd.SetContext(context.Background())
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	err := run(rootCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "environment validation failed")
}

// TestRun_GuardPolicyOverrideFromFlags verifies that run() applies a guard
// policy override resolved from CLI flags (--guard-policy-json) onto the
// loaded config, exercising the policyOverride != nil branch, and starts
// successfully with the override configured.
func TestRun_GuardPolicyOverrideFromFlags(t *testing.T) {
	resetRunFlagsForTest(t)
	resetGuardPolicyFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	require.NoError(t, rootCmd.Flags().Set("guard-policy-json", `{"allow-only":{"repos":"public","min-integrity":"none"}}`))

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(rootCmd, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should start successfully with a guard policy override from CLI flags")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

// TestRun_InvalidGuardPolicyOverrideFromFlags verifies that run() surfaces a
// wrapped error when --guard-policy-json contains invalid JSON, exercising
// the resolveGuardPolicyFromFlags error-return branch.
func TestRun_InvalidGuardPolicyOverrideFromFlags(t *testing.T) {
	resetRunFlagsForTest(t)
	resetGuardPolicyFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	require.NoError(t, rootCmd.Flags().Set("guard-policy-json", "not-valid-json"))

	rootCmd.SetContext(context.Background())
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	err := run(rootCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid guard policy configuration")
}

// TestRun_SinkServerIDsConfiguredWithUnknownServer verifies that run() parses
// --guards-sink-server-ids, sets the DIFC sink server IDs, and logs a warning
// (without failing) when a configured sink server ID does not match any
// configured backend server. This exercises the non-empty resolvedSinkServerIDs
// branch and its per-ID "not configured" warning path.
func TestRun_SinkServerIDsConfiguredWithUnknownServer(t *testing.T) {
	resetRunFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"
	difcSinkServerIDs = "testserver,unknown-server"

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(rootCmd, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should start successfully even when a sink server ID is unrecognized")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

// TestRun_InvalidSinkServerIDs verifies that run() surfaces a wrapped error
// when --guards-sink-server-ids contains an invalid value (whitespace),
// exercising the difc.ParseSinkServerIDs error-return branch.
func TestRun_InvalidSinkServerIDs(t *testing.T) {
	resetRunFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"
	difcSinkServerIDs = "has whitespace"

	rootCmd.SetContext(context.Background())
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	err := run(rootCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --guards-sink-server-ids value")
}

// TestRun_SequentialLaunchEnabled verifies that run() takes the sequential
// launch log branch and starts successfully when --sequential-launch is set.
func TestRun_SequentialLaunchEnabled(t *testing.T) {
	resetRunFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"
	sequentialLaunch = true

	require.NoError(t, rootCmd.Flags().Set("sequential-launch", "true"))
	t.Cleanup(func() { _ = rootCmd.Flags().Set("sequential-launch", "false") })

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(rootCmd, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should start successfully with sequential launch enabled")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

// TestRun_TLSEnabled verifies that run() takes the TLS listener branch: it
// generates/uses a self-signed certificate pair, wraps the TCP listener with
// TLS, and shuts down gracefully. This exercises the tlsEnabled=true path of
// setupTLSListener as invoked from run().
func TestRun_TLSEnabled(t *testing.T) {
	resetRunFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	tlsDir := t.TempDir()
	tlsCfg, err := proxy.GenerateSelfSignedTLS(tlsDir)
	require.NoError(t, err)
	tlsCertPath = tlsCfg.CertPath
	tlsKeyPath = tlsCfg.KeyPath

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(rootCmd, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should start successfully with TLS enabled")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

// TestRun_HMACSecretEnabled verifies that run() logs the HMAC request-signing
// enabled message and starts successfully when --hmac-secret is configured.
func TestRun_HMACSecretEnabled(t *testing.T) {
	resetRunFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"
	hmacSecret = "a-very-long-shared-hmac-secret-value-1234567890"

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(rootCmd, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should start successfully with HMAC secret configured")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

// TestRun_InvalidTLSConfiguration verifies that run() surfaces a wrapped error
// from setupTLSListener when only --tls-cert is set without --tls-key,
// exercising the setupTLSListener error-return branch inside run().
func TestRun_InvalidTLSConfiguration(t *testing.T) {
	resetRunFlagsForTest(t)

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"
	tlsCertPath = filepath.Join(t.TempDir(), "cert.pem")
	tlsKeyPath = ""

	rootCmd.SetContext(context.Background())
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	err := run(rootCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--tls-cert and --tls-key must both be provided together")
}

// TestRun_PartialDelegationEnvVars verifies that run() surfaces a wrapped
// error from resolveDelegationProxyConfig when only some of the
// MCP_GATEWAY_DELEGATION_* environment variables are set, exercising the
// delegation-config error-return branch inside run().
func TestRun_PartialDelegationEnvVars(t *testing.T) {
	resetRunFlagsForTest(t)

	t.Setenv("MCP_GATEWAY_DELEGATION_ENVELOPE", `{"runId":"run-1"}`)
	t.Setenv("MCP_GATEWAY_DELEGATION_STATE_PATH", "")
	t.Setenv("MCP_GATEWAY_DELEGATION_GENERATION", "")
	// Intentionally leave capability key and control listen addr env vars unset.

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	rootCmd.SetContext(context.Background())
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	err := run(rootCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be configured together")
}

// TestRun_SinkServerIDsEnvVarLogged verifies that run() logs the resolved
// value of MCP_GATEWAY_GUARDS_SINK_SERVER_IDS when the environment variable is
// set, exercising the os.LookupEnv "exists" branch that is independent of the
// --guards-sink-server-ids CLI flag.
func TestRun_SinkServerIDsEnvVarLogged(t *testing.T) {
	resetRunFlagsForTest(t)

	t.Setenv("MCP_GATEWAY_GUARDS_SINK_SERVER_IDS", "testserver")

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"
	difcSinkServerIDs = ""

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(rootCmd, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should start successfully when MCP_GATEWAY_GUARDS_SINK_SERVER_IDS env var is set")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

// TestRun_OTLPTracingEnabled verifies that run() takes the tracing-enabled
// logging branch (tracingProvider.IsEnabled() == true) when --otlp-endpoint is
// configured, and starts/shuts down successfully. The exporter never actually
// flushes any spans over the network within the short test window, so no real
// OTLP collector is required.
func TestRun_OTLPTracingEnabled(t *testing.T) {
	resetRunFlagsForTest(t)

	origOTLPEndpoint := otlpEndpoint
	t.Cleanup(func() {
		otlpEndpoint = origOTLPEndpoint
	})

	configFile = writeMinimalConfigFile(t)
	configStdin = false
	envFile = ""
	listenAddr = "127.0.0.1:0"
	routedMode = true
	unifiedMode = false
	logDir = t.TempDir()
	wasmCacheDir = t.TempDir()
	shutdownTimeout = 2 * time.Second
	difcMode = "strict"

	// applyTracingOverrides reads the otlpEndpoint global directly (populated
	// normally via registerTracingFlags/--otlp-endpoint); set it here since
	// that flag isn't registered on the plain rootCmd used by these tests.
	otlpEndpoint = "http://127.0.0.1:1/v1/traces"

	ctx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(ctx)
	t.Cleanup(func() { rootCmd.SetContext(context.Background()) })

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(rootCmd, nil)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should start successfully with OTLP tracing enabled")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

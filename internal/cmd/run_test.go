package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		assert.NoError(t, err, "run() should shut down gracefully without error when context is cancelled")
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
		assert.NoError(t, err, "run() should start successfully with only gateway.agentIds configured")
	case <-time.After(5 * time.Second):
		t.Fatal("run() did not return within the expected shutdown window")
	}
}

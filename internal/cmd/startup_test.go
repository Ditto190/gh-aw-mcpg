package cmd

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/github/gh-aw-mcpg/internal/proxy"
	"github.com/github/gh-aw-mcpg/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupTLSListener_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		certPath    string
		keyPath     string
		caPath      string
		errContains string
	}{
		{
			name:        "cert without key",
			certPath:    "/tmp/server.crt",
			errContains: "--tls-cert and --tls-key must both be provided together",
		},
		{
			name:        "key without cert",
			keyPath:     "/tmp/server.key",
			errContains: "--tls-cert and --tls-key must both be provided together",
		},
		{
			name:        "ca without cert and key",
			caPath:      "/tmp/ca.crt",
			errContains: "--tls-ca requires --tls-cert and --tls-key to also be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			listener, tlsEnabled, err := setupTLSListener("127.0.0.1:0", tt.certPath, tt.keyPath, tt.caPath)
			require.Error(t, err)
			assert.Nil(t, listener)
			assert.False(t, tlsEnabled)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestSetupTLSListener_SuccessCases(t *testing.T) {
	t.Parallel()

	certsDir := t.TempDir()
	tlsCfg, err := proxy.GenerateSelfSignedTLS(certsDir)
	require.NoError(t, err)

	tests := []struct {
		name      string
		certPath  string
		keyPath   string
		caPath    string
		tlsEnable bool
	}{
		{
			name:      "plain http listener",
			tlsEnable: false,
		},
		{
			name:      "tls listener",
			certPath:  tlsCfg.CertPath,
			keyPath:   tlsCfg.KeyPath,
			tlsEnable: true,
		},
		{
			name:      "mtls listener",
			certPath:  tlsCfg.CertPath,
			keyPath:   tlsCfg.KeyPath,
			caPath:    tlsCfg.CACertPath,
			tlsEnable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			listenAddr := availableLoopbackAddr(t)
			listener, tlsEnabled, err := setupTLSListener(listenAddr, tt.certPath, tt.keyPath, tt.caPath)
			require.NoError(t, err)
			require.NotNil(t, listener)
			assert.Equal(t, tt.tlsEnable, tlsEnabled)
			assert.NoError(t, listener.Close())
		})
	}
}

func TestSetupTLSListener_ClosesListenerOnTLSFailure(t *testing.T) {
	t.Parallel()

	listenAddr := availableLoopbackAddr(t)
	missingDir := t.TempDir()
	missingCert := filepath.Join(missingDir, "missing.crt")
	missingKey := filepath.Join(missingDir, "missing.key")

	listener, tlsEnabled, err := setupTLSListener(listenAddr, missingCert, missingKey, "")
	require.Error(t, err)
	assert.Nil(t, listener)
	assert.False(t, tlsEnabled)
	assert.Contains(t, err.Error(), "failed to configure TLS")

	rebound, reboundErr := net.Listen("tcp", listenAddr)
	require.NoError(t, reboundErr, "listener should be closed on TLS setup failure")
	assert.NoError(t, rebound.Close())
}

// newTestUnifiedServer builds a minimal UnifiedServer with no backend servers
// configured, suitable for exercising HTTP-server wiring without Docker.
func newTestUnifiedServer(t *testing.T) *server.UnifiedServer {
	t.Helper()
	cfg := &config.Config{
		Servers: map[string]*config.ServerConfig{},
	}
	us, err := server.NewUnified(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { us.Close() })
	return us
}

func TestBuildHTTPServer_RoutedMode(t *testing.T) {
	t.Parallel()

	us := newTestUnifiedServer(t)
	ctx := context.Background()
	listenAddr := availableLoopbackAddr(t)

	httpServer := buildHTTPServer(ctx, "routed", listenAddr, us, "test-agent-id", "", func() {})

	require.NotNil(t, httpServer)
	assert.Equal(t, listenAddr, httpServer.Addr)
	require.NotNil(t, httpServer.BaseContext)
	assert.Equal(t, ctx, httpServer.BaseContext(nil))
}

func TestBuildHTTPServer_UnifiedMode(t *testing.T) {
	t.Parallel()

	us := newTestUnifiedServer(t)
	ctx := context.Background()
	listenAddr := availableLoopbackAddr(t)

	cancelCalled := false
	cancel := func() { cancelCalled = true }

	httpServer := buildHTTPServer(ctx, "unified", listenAddr, us, "", "", cancel)

	require.NotNil(t, httpServer)
	assert.Equal(t, listenAddr, httpServer.Addr)
	require.NotNil(t, httpServer.BaseContext)
	assert.Equal(t, ctx, httpServer.BaseContext(nil))

	// buildHTTPServer must wire the cancel func into the unified server's exit
	// hook so that the /close handler can invoke it during shutdown.
	exitFn := us.GetExitFunc()
	require.NotNil(t, exitFn, "expected buildHTTPServer to register an exit function")
	exitFn()
	assert.True(t, cancelCalled, "expected cancel to be invoked via the registered exit hook")
}

func TestBuildHTTPServer_BaseContextPropagatesAcrossRequests(t *testing.T) {
	t.Parallel()

	us := newTestUnifiedServer(t)
	type ctxKey string
	key := ctxKey("test-key")
	ctx := context.WithValue(context.Background(), key, "test-value")
	listenAddr := availableLoopbackAddr(t)

	httpServer := buildHTTPServer(ctx, "unified", listenAddr, us, "", "", func() {})
	require.NotNil(t, httpServer.BaseContext)

	gotCtx := httpServer.BaseContext(nil)
	assert.Equal(t, "test-value", gotCtx.Value(key))
}

func TestBuildHTTPServer_WithHMACSecret(t *testing.T) {
	t.Parallel()

	us := newTestUnifiedServer(t)
	ctx := context.Background()
	listenAddr := availableLoopbackAddr(t)

	httpServer := buildHTTPServer(ctx, "routed", listenAddr, us, "agent-id", "hmac-secret", func() {})
	require.NotNil(t, httpServer)
	assert.Equal(t, listenAddr, httpServer.Addr)
}

// ensure the returned server actually serves requests via the unified server's
// mux (sanity check that CreateHTTPServerForRoutedMode/CreateHTTPServerForMCP
// wiring is intact and buildHTTPServer doesn't break the handler).
func TestBuildHTTPServer_ServesRequests(t *testing.T) {
	t.Parallel()

	us := newTestUnifiedServer(t)
	ctx := context.Background()
	listenAddr := availableLoopbackAddr(t)

	httpServer := buildHTTPServer(ctx, "unified", listenAddr, us, "", "", func() {})
	require.NotNil(t, httpServer)

	ln, err := net.Listen("tcp", listenAddr)
	require.NoError(t, err)
	go func() { _ = httpServer.Serve(ln) }()
	defer func() { _ = httpServer.Close() }()

	// Give the server a brief moment to start accepting connections.
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + listenAddr + "/nonexistent")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return true
	}, 2*time.Second, 50*time.Millisecond)
}

func availableLoopbackAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

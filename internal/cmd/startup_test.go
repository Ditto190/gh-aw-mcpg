package cmd

import (
	"net"
	"path/filepath"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/proxy"
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

func availableLoopbackAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSelfSignedTLS(t *testing.T) {
	t.Run("generates valid CA and server certificates", func(t *testing.T) {
		dir := t.TempDir()
		tlsCfg, err := GenerateSelfSignedTLS(dir)
		require.NoError(t, err)
		require.NotNil(t, tlsCfg)

		// Files exist
		assert.FileExists(t, tlsCfg.CACertPath)
		assert.FileExists(t, tlsCfg.CertPath)
		assert.FileExists(t, tlsCfg.KeyPath)

		// Paths are in the expected directory
		assert.Equal(t, filepath.Join(dir, "ca.crt"), tlsCfg.CACertPath)
		assert.Equal(t, filepath.Join(dir, "server.crt"), tlsCfg.CertPath)
		assert.Equal(t, filepath.Join(dir, "server.key"), tlsCfg.KeyPath)

		// tls.Config is populated
		require.NotNil(t, tlsCfg.Config)
		assert.Len(t, tlsCfg.Config.Certificates, 1)
		assert.Equal(t, uint16(tls.VersionTLS12), tlsCfg.Config.MinVersion)
	})

	t.Run("CA cert is trusted for server cert", func(t *testing.T) {
		dir := t.TempDir()
		tlsCfg, err := GenerateSelfSignedTLS(dir)
		require.NoError(t, err)

		// Load CA cert into a pool
		caCertPEM, err := os.ReadFile(tlsCfg.CACertPath)
		require.NoError(t, err)

		caPool := x509.NewCertPool()
		ok := caPool.AppendCertsFromPEM(caCertPEM)
		require.True(t, ok, "CA cert should be parseable PEM")

		// Start a TLS server with the generated config
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		srv.TLS = tlsCfg.Config
		srv.StartTLS()
		defer srv.Close()

		// Client trusting only our CA should connect successfully
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: caPool,
				},
			},
		}

		resp, err := client.Get(srv.URL)
		require.NoError(t, err, "TLS handshake should succeed with CA trust")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("untrusted client rejects server cert", func(t *testing.T) {
		dir := t.TempDir()
		tlsCfg, err := GenerateSelfSignedTLS(dir)
		require.NoError(t, err)

		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		srv.TLS = tlsCfg.Config
		srv.StartTLS()
		defer srv.Close()

		// Client with default (system) trust store should reject the cert
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{},
			},
		}

		_, err = client.Get(srv.URL)
		require.Error(t, err, "TLS handshake should fail without CA trust")
		assert.ErrorContains(t, err, "certificate")
	})

	t.Run("server cert covers localhost, 127.0.0.1, and ::1", func(t *testing.T) {
		dir := t.TempDir()
		tlsCfg, err := GenerateSelfSignedTLS(dir)
		require.NoError(t, err)

		// Parse the server certificate
		leaf, err := x509.ParseCertificate(tlsCfg.Config.Certificates[0].Certificate[0])
		require.NoError(t, err)

		assert.Contains(t, leaf.DNSNames, "localhost")
		foundLoopback4 := false
		foundLoopback6 := false
		for _, ip := range leaf.IPAddresses {
			if ip.Equal(net.IPv4(127, 0, 0, 1)) {
				foundLoopback4 = true
			}
			if ip.Equal(net.IPv6loopback) {
				foundLoopback6 = true
			}
		}
		assert.True(t, foundLoopback4, "server cert should cover 127.0.0.1")
		assert.True(t, foundLoopback6, "server cert should cover ::1")
	})

	t.Run("key files have restricted permissions", func(t *testing.T) {
		dir := t.TempDir()
		tlsCfg, err := GenerateSelfSignedTLS(dir)
		require.NoError(t, err)

		info, err := os.Stat(tlsCfg.KeyPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "private key should be owner-only")
	})

	t.Run("creates directory if missing", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nested", "subdir")
		_, err := GenerateSelfSignedTLS(dir)
		require.NoError(t, err)
		assert.DirExists(t, dir)
	})

	t.Run("certificates are short-lived (24h)", func(t *testing.T) {
		dir := t.TempDir()
		tlsCfg, err := GenerateSelfSignedTLS(dir)
		require.NoError(t, err)

		leaf, err := x509.ParseCertificate(tlsCfg.Config.Certificates[0].Certificate[0])
		require.NoError(t, err)

		validity := leaf.NotAfter.Sub(leaf.NotBefore)
		assert.InDelta(t, 25*3600, validity.Seconds(), 3600, "cert validity should be ~25h (24h + 1h backdate)")
	})

	t.Run("returns error when directory cannot be created", func(t *testing.T) {
		// /dev/null is a character device, so any subdirectory under it cannot be created.
		dir := "/dev/null/cannot-create-this"
		tlsCfg, err := GenerateSelfSignedTLS(dir)
		require.Error(t, err, "should fail when the directory cannot be created")
		assert.Nil(t, tlsCfg)
		assert.ErrorContains(t, err, "failed to create TLS directory")
	})

	t.Run("CA cert public key matches CA key in server cert issuer", func(t *testing.T) {
		dir := t.TempDir()
		tlsCfg, err := GenerateSelfSignedTLS(dir)
		require.NoError(t, err)

		caCertPEM, err := os.ReadFile(tlsCfg.CACertPath)
		require.NoError(t, err)
		caPool := x509.NewCertPool()
		require.True(t, caPool.AppendCertsFromPEM(caCertPEM), "CA cert should be parseable PEM")

		serverCert, err := x509.ParseCertificate(tlsCfg.Config.Certificates[0].Certificate[0])
		require.NoError(t, err)

		// Verify server cert chains to our CA
		opts := x509.VerifyOptions{
			DNSName: "localhost",
			Roots:   caPool,
		}
		_, err = serverCert.Verify(opts)
		assert.NoError(t, err, "server cert should verify against the generated CA")
	})

	t.Run("server cert has expected issuer fields", func(t *testing.T) {
		dir := t.TempDir()
		tlsCfg, err := GenerateSelfSignedTLS(dir)
		require.NoError(t, err)

		serverCert, err := x509.ParseCertificate(tlsCfg.Config.Certificates[0].Certificate[0])
		require.NoError(t, err)

		assert.Equal(t, "MCPG Proxy CA", serverCert.Issuer.CommonName)
		require.NotEmpty(t, serverCert.Issuer.Organization)
		assert.Equal(t, "MCPG Proxy", serverCert.Issuer.Organization[0])
	})
}

// TestWritePEM_InvalidPath verifies that writePEM returns an error when the
// parent directory does not exist.
func TestWritePEM_InvalidPath(t *testing.T) {
	// Use a path whose parent directory does not exist.
	path := filepath.Join(t.TempDir(), "nonexistent-subdir", "file.pem")
	err := writePEM(path, "CERTIFICATE", []byte("dummy"), 0644)
	require.Error(t, err, "writePEM should fail when the parent directory does not exist")
}

// TestRandomSerial_ReturnsPositive verifies that randomSerial always generates
// a positive integer (i.e. the serial number is never zero).
func TestRandomSerial_ReturnsPositive(t *testing.T) {
	for i := 0; i < 20; i++ {
		serial, err := randomSerial()
		require.NoError(t, err, "randomSerial should not fail")
		assert.Positive(t, serial.Sign(), "serial should always be positive")
		// 128-bit serial: must be less than 2^128
		assert.LessOrEqual(t, serial.BitLen(), 128, "serial should fit in 128 bits")
	}
}

package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
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
		parent := t.TempDir()
		dir := filepath.Join(parent, "not-a-directory")
		require.NoError(t, os.WriteFile(dir, []byte("block directory creation"), 0644))

		tlsCfg, err := GenerateSelfSignedTLS(dir)
		require.Error(t, err, "should fail when the directory cannot be created")
		assert.Nil(t, tlsCfg)
		assert.ErrorContains(t, err, "failed to create TLS directory")
	})

	t.Run("server cert verifies against generated CA", func(t *testing.T) {
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

// TestGenerateSelfSignedTLS_WritePEMErrors exercises the writePEM failure
// branches inside GenerateSelfSignedTLS by pre-creating each target file as a
// directory, which forces os.OpenFile to fail with "is a directory".
func TestGenerateSelfSignedTLS_WritePEMErrors(t *testing.T) {
	tests := []struct {
		name        string
		blockedFile string
		wantErrMsg  string
	}{
		{
			name:        "ca.crt write failure",
			blockedFile: "ca.crt",
			wantErrMsg:  "failed to write CA cert",
		},
		{
			name:        "server.crt write failure",
			blockedFile: "server.crt",
			wantErrMsg:  "failed to write server cert",
		},
		{
			name:        "server.key write failure",
			blockedFile: "server.key",
			wantErrMsg:  "failed to write server key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			// Pre-create the target file as a directory so writePEM's
			// os.OpenFile call fails with "is a directory".
			require.NoError(t, os.MkdirAll(filepath.Join(dir, tt.blockedFile), 0755))

			tlsCfg, err := GenerateSelfSignedTLS(dir)
			require.Error(t, err)
			assert.Nil(t, tlsCfg)
			assert.ErrorContains(t, err, tt.wantErrMsg)
		})
	}
}

// TestWritePEM_Success verifies that writePEM writes a well-formed PEM file
// to disk with the correct block type, permissions, and content, exercising
// the full happy path (open, encode, close, and the final success log).
func TestWritePEM_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cert.pem")
	derBytes := []byte("dummy DER bytes for testing purposes")

	err := writePEM(path, "CERTIFICATE", derBytes, 0600)
	require.NoError(t, err, "writePEM should succeed for a valid path and writable directory")

	info, statErr := os.Stat(path)
	require.NoError(t, statErr, "the PEM file should exist after writePEM succeeds")
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "file should be created with the requested permissions")

	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Contains(t, string(content), "-----BEGIN CERTIFICATE-----")
	assert.Contains(t, string(content), "-----END CERTIFICATE-----")

	// Decoding back should recover the original DER bytes and block type.
	block, _ := decodePEMBlock(t, content)
	require.NotNil(t, block)
	assert.Equal(t, "CERTIFICATE", block.Type)
	assert.Equal(t, derBytes, block.Bytes)
}

// TestWritePEM_TruncatesExistingFile verifies that writePEM truncates and
// overwrites a pre-existing file rather than appending to it (os.O_TRUNC).
func TestWritePEM_TruncatesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cert.pem")

	// Pre-create the file with content much longer than the new content,
	// so a failure to truncate would leave stale trailing bytes.
	require.NoError(t, os.WriteFile(path, []byte(
		"-----BEGIN OLD-----\nAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n-----END OLD-----\n"), 0644))

	err := writePEM(path, "CERTIFICATE", []byte("new"), 0644)
	require.NoError(t, err)

	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.NotContains(t, string(content), "OLD", "old content should be truncated, not appended to")
	assert.Contains(t, string(content), "CERTIFICATE")
}

// TestWritePEM_EncodeFailure exercises the pem.Encode failure branch (and the
// subsequent best-effort Close on the error path) by writing to /dev/full,
// a special device that always reports "no space left on device" on write.
// This branch was previously uncovered because ordinary filesystem writes
// essentially never fail once the file has been opened successfully.
func TestWritePEM_EncodeFailure(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("/dev/full is not available on this platform")
	}

	err := writePEM("/dev/full", "CERTIFICATE", []byte("dummy DER payload padding padding padding"), 0644)
	require.Error(t, err, "writePEM should surface the pem.Encode error when the underlying write fails")
	assert.Contains(t, err.Error(), "no space left on device")
}

// decodePEMBlock is a small test helper that decodes the first PEM block from
// data, failing the test if no block is found.
func decodePEMBlock(t *testing.T, data []byte) (*pem.Block, []byte) {
	t.Helper()
	block, rest := pem.Decode(data)
	require.NotNil(t, block, "expected data to contain a valid PEM block")
	return block, rest
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

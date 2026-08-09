package httputil_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/github/gh-aw-mcpg/internal/httputil"
	"github.com/github/gh-aw-mcpg/internal/proxy"
)

// generateClientCert creates an ephemeral client certificate signed by the CA
// whose cert is at caCertPath and whose private key is at caKeyPath.
// It returns a tls.Certificate with ExtKeyUsageClientAuth. Any failure fails
// the test immediately via require, so callers don't need to handle errors.
func generateClientCert(t *testing.T, dir, caCertPath, caKeyPath string) tls.Certificate {
	t.Helper()

	caPEM, err := os.ReadFile(caCertPath)
	require.NoError(t, err)
	block, _ := pem.Decode(caPEM)
	require.NotNil(t, block, "failed to decode CA cert PEM")
	caCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	caKeyPEM, err := os.ReadFile(caKeyPath)
	require.NoError(t, err)
	keyBlock, _ := pem.Decode(caKeyPEM)
	require.NotNil(t, keyBlock, "failed to decode CA key PEM")
	caKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	require.NoError(t, err)

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &clientKey.PublicKey, caKey)
	require.NoError(t, err)

	certPath := filepath.Join(dir, "client.crt")
	writePEMFile(t, certPath, "CERTIFICATE", certDER, 0644)

	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	require.NoError(t, err)
	keyPath := filepath.Join(dir, "client.key")
	writePEMFile(t, keyPath, "EC PRIVATE KEY", clientKeyDER, 0600)

	clientTLSCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err)
	return clientTLSCert
}

// writePEMFile writes a DER-encoded block as PEM to path with the given mode,
// failing the test immediately if the write fails.
func writePEMFile(t *testing.T, path, blockType string, derBytes []byte, mode os.FileMode) {
	t.Helper()
	data := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: derBytes})
	require.NoError(t, os.WriteFile(path, data, mode))
}

// mtlsCerts bundles the CA, server, and client certificates generated for mTLS tests.
// The server cert has ExtKeyUsageServerAuth; the client cert has ExtKeyUsageClientAuth.
type mtlsCerts struct {
	caCertPath     string
	serverCertPath string
	serverKeyPath  string
	clientCert     tls.Certificate
	caPool         *x509.CertPool
}

// generateMTLSCerts generates a CA, server cert, and client cert for mTLS tests,
// failing the test immediately via require on any error.
func generateMTLSCerts(t *testing.T, dir string) *mtlsCerts {
	t.Helper()

	// --- CA ---
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caCertDER)
	require.NoError(t, err)

	caCertPath := filepath.Join(dir, "ca.crt")
	writePEMFile(t, caCertPath, "CERTIFICATE", caCertDER, 0644)

	caKeyDER, err := x509.MarshalECPrivateKey(caKey)
	require.NoError(t, err)
	caKeyPath := filepath.Join(dir, "ca.key")
	writePEMFile(t, caKeyPath, "EC PRIVATE KEY", caKeyDER, 0600)

	// --- Server cert ---
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serverSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	require.NoError(t, err)
	serverCertPath := filepath.Join(dir, "server.crt")
	writePEMFile(t, serverCertPath, "CERTIFICATE", serverCertDER, 0644)

	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	require.NoError(t, err)
	serverKeyPath := filepath.Join(dir, "server.key")
	writePEMFile(t, serverKeyPath, "EC PRIVATE KEY", serverKeyDER, 0600)

	// --- Client cert ---
	clientTLSCert := generateClientCert(t, dir, caCertPath, caKeyPath)

	caPool := x509.NewCertPool()
	caPEM, err := os.ReadFile(caCertPath)
	require.NoError(t, err)
	require.True(t, caPool.AppendCertsFromPEM(caPEM), "failed to append CA cert to pool")

	return &mtlsCerts{
		caCertPath:     caCertPath,
		serverCertPath: serverCertPath,
		serverKeyPath:  serverKeyPath,
		clientCert:     clientTLSCert,
		caPool:         caPool,
	}
}

func TestLoadGatewayTLS_ServerOnly(t *testing.T) {
	dir := t.TempDir()
	tlsCfg, err := proxy.GenerateSelfSignedTLS(dir)
	require.NoError(t, err)

	cfg, err := httputil.LoadGatewayTLS(tlsCfg.CertPath, tlsCfg.KeyPath, "")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Len(t, cfg.Certificates, 1, "should load one certificate")
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.Equal(t, tls.NoClientCert, cfg.ClientAuth, "server-only TLS should not require client certs")
	assert.Nil(t, cfg.ClientCAs, "server-only TLS should have no CA pool")
}

func TestLoadGatewayTLS_MutualTLS(t *testing.T) {
	dir := t.TempDir()
	tlsCfg, err := proxy.GenerateSelfSignedTLS(dir)
	require.NoError(t, err)

	cfg, err := httputil.LoadGatewayTLS(tlsCfg.CertPath, tlsCfg.KeyPath, tlsCfg.CACertPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth, "mTLS should require client certs")
	assert.NotNil(t, cfg.ClientCAs, "mTLS should populate CA pool")
}

func TestLoadGatewayTLS_ServerServesMTLS(t *testing.T) {
	dir := t.TempDir()
	certs := generateMTLSCerts(t, dir)

	cfg, err := httputil.LoadGatewayTLS(certs.serverCertPath, certs.serverKeyPath, certs.caCertPath)
	require.NoError(t, err)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)

	t.Run("valid client cert succeeds", func(t *testing.T) {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:      certs.caPool,
					Certificates: []tls.Certificate{certs.clientCert},
				},
			},
		}
		resp, err := client.Get(srv.URL)
		require.NoError(t, err, "mTLS handshake should succeed with valid client cert")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("missing client cert is rejected", func(t *testing.T) {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: certs.caPool,
					// No client certificate presented.
				},
			},
		}
		_, err := client.Get(srv.URL)
		assert.Error(t, err, "server should reject connections without a client certificate")
	})
}

func TestLoadGatewayTLS_InvalidCertPath(t *testing.T) {
	tests := []struct {
		name     string
		certPath string
		keyPath  string
		caPath   string
		wantErr  string
	}{
		{
			name:     "nonexistent cert and key",
			certPath: "/nonexistent/cert.pem",
			keyPath:  "/nonexistent/key.pem",
			caPath:   "",
			wantErr:  "failed to load server TLS certificate/key",
		},
		{
			name:     "empty cert and key paths",
			certPath: "",
			keyPath:  "",
			caPath:   "",
			wantErr:  "failed to load server TLS certificate/key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := httputil.LoadGatewayTLS(tt.certPath, tt.keyPath, tt.caPath)
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestLoadGatewayTLS_InvalidCAPath(t *testing.T) {
	dir := t.TempDir()
	tlsCfg, err := proxy.GenerateSelfSignedTLS(dir)
	require.NoError(t, err)

	cfg, err := httputil.LoadGatewayTLS(tlsCfg.CertPath, tlsCfg.KeyPath, "/nonexistent/ca.pem")
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "failed to read CA certificate")
}

func TestLoadGatewayTLS_MalformedCA(t *testing.T) {
	dir := t.TempDir()
	tlsCfg, err := proxy.GenerateSelfSignedTLS(dir)
	require.NoError(t, err)

	tests := []struct {
		name    string
		content string
	}{
		{name: "garbage text", content: "NOT A VALID PEM"},
		{name: "empty file", content: ""},
		{name: "valid PEM header but invalid DER body", content: "-----BEGIN CERTIFICATE-----\nbm90IHZhbGlk\n-----END CERTIFICATE-----\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			badCA := filepath.Join(dir, "bad-ca-"+tt.name+".pem")
			require.NoError(t, os.WriteFile(badCA, []byte(tt.content), 0644))

			cfg, err := httputil.LoadGatewayTLS(tlsCfg.CertPath, tlsCfg.KeyPath, badCA)
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.ErrorContains(t, err, "failed to parse CA certificate")
		})
	}
}

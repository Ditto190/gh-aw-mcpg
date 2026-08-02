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
// It returns a tls.Certificate with ExtKeyUsageClientAuth.
//
// Every intermediate step is fatal on failure via require, since this is a
// test-only helper and there is no meaningful way to continue the test if any
// step fails.
func generateClientCert(t *testing.T, dir, caCertPath, caKeyPath string) tls.Certificate {
	t.Helper()

	caPEM, err := os.ReadFile(caCertPath)
	require.NoError(t, err, "reading CA certificate")
	block, _ := pem.Decode(caPEM)
	require.NotNil(t, block, "decoding CA certificate PEM block")
	caCert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err, "parsing CA certificate")

	caKeyPEM, err := os.ReadFile(caKeyPath)
	require.NoError(t, err, "reading CA private key")
	keyBlock, _ := pem.Decode(caKeyPEM)
	require.NotNil(t, keyBlock, "decoding CA private key PEM block")
	caKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	require.NoError(t, err, "parsing CA private key")

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generating client private key")

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err, "generating client cert serial number")

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &clientKey.PublicKey, caKey)
	require.NoError(t, err, "creating client certificate")

	certPath := filepath.Join(dir, "client.crt")
	require.NoError(t, writePEMFile(certPath, "CERTIFICATE", certDER, 0644), "writing client certificate")

	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	require.NoError(t, err, "marshaling client private key")
	keyPath := filepath.Join(dir, "client.key")
	require.NoError(t, writePEMFile(keyPath, "EC PRIVATE KEY", clientKeyDER, 0600), "writing client private key")

	clientTLSCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err, "loading client key pair")
	return clientTLSCert
}

// writePEMFile writes a DER-encoded block as PEM to path with the given mode.
func writePEMFile(path, blockType string, derBytes []byte, mode os.FileMode) error {
	data := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: derBytes})
	return os.WriteFile(path, data, mode)
}

// generateMTLSCerts generates a CA, server cert, and client cert for mTLS tests.
// The server cert has ExtKeyUsageServerAuth; the client cert has ExtKeyUsageClientAuth.
type mtlsCerts struct {
	caCertPath     string
	serverCertPath string
	serverKeyPath  string
	clientCert     tls.Certificate
	caPool         *x509.CertPool
}

// generateMTLSCerts fails the test immediately (via require) if any
// certificate-generation step fails, since this is test-only scaffolding
// with no meaningful recovery path.
func generateMTLSCerts(t *testing.T, dir string) *mtlsCerts {
	t.Helper()

	// --- CA ---
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generating CA private key")
	caSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err, "generating CA serial number")
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
	require.NoError(t, err, "creating CA certificate")
	caCert, err := x509.ParseCertificate(caCertDER)
	require.NoError(t, err, "parsing CA certificate")

	caCertPath := filepath.Join(dir, "ca.crt")
	require.NoError(t, writePEMFile(caCertPath, "CERTIFICATE", caCertDER, 0644), "writing CA certificate")

	caKeyDER, err := x509.MarshalECPrivateKey(caKey)
	require.NoError(t, err, "marshaling CA private key")
	caKeyPath := filepath.Join(dir, "ca.key")
	require.NoError(t, writePEMFile(caKeyPath, "EC PRIVATE KEY", caKeyDER, 0600), "writing CA private key")

	// --- Server cert ---
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "generating server private key")
	serverSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err, "generating server serial number")
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
	require.NoError(t, err, "creating server certificate")
	serverCertPath := filepath.Join(dir, "server.crt")
	require.NoError(t, writePEMFile(serverCertPath, "CERTIFICATE", serverCertDER, 0644), "writing server certificate")

	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	require.NoError(t, err, "marshaling server private key")
	serverKeyPath := filepath.Join(dir, "server.key")
	require.NoError(t, writePEMFile(serverKeyPath, "EC PRIVATE KEY", serverKeyDER, 0600), "writing server private key")

	// --- Client cert ---
	clientTLSCert := generateClientCert(t, dir, caCertPath, caKeyPath)

	caPool := x509.NewCertPool()
	caPEM, err := os.ReadFile(caCertPath)
	require.NoError(t, err, "reading CA certificate for pool")
	require.True(t, caPool.AppendCertsFromPEM(caPEM), "appending CA certificate to pool")

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
	defer srv.Close()

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
}

// TestLoadGatewayTLS_ErrorPaths exercises every distinct error branch of
// LoadGatewayTLS with a table-driven set of malformed inputs, verifying both
// that an error is returned and that its message identifies the failing step.
func TestLoadGatewayTLS_ErrorPaths(t *testing.T) {
	dir := t.TempDir()
	tlsCfg, err := proxy.GenerateSelfSignedTLS(dir)
	require.NoError(t, err)

	badCA := filepath.Join(dir, "bad-ca.pem")
	require.NoError(t, os.WriteFile(badCA, []byte("NOT A VALID PEM"), 0644))

	emptyCA := filepath.Join(dir, "empty-ca.pem")
	require.NoError(t, os.WriteFile(emptyCA, []byte{}, 0644))

	tests := []struct {
		name        string
		certPath    string
		keyPath     string
		caPath      string
		wantErrText string
	}{
		{
			name:        "nonexistent server cert/key",
			certPath:    "/nonexistent/cert.pem",
			keyPath:     "/nonexistent/key.pem",
			caPath:      "",
			wantErrText: "failed to load server TLS certificate/key",
		},
		{
			name:        "mismatched key file",
			certPath:    tlsCfg.CertPath,
			keyPath:     "/nonexistent/key.pem",
			caPath:      "",
			wantErrText: "failed to load server TLS certificate/key",
		},
		{
			name:        "nonexistent CA file",
			certPath:    tlsCfg.CertPath,
			keyPath:     tlsCfg.KeyPath,
			caPath:      "/nonexistent/ca.pem",
			wantErrText: "failed to read CA certificate",
		},
		{
			name:        "malformed CA PEM",
			certPath:    tlsCfg.CertPath,
			keyPath:     tlsCfg.KeyPath,
			caPath:      badCA,
			wantErrText: "failed to parse CA certificate",
		},
		{
			name:        "empty CA file",
			certPath:    tlsCfg.CertPath,
			keyPath:     tlsCfg.KeyPath,
			caPath:      emptyCA,
			wantErrText: "failed to parse CA certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := httputil.LoadGatewayTLS(tt.certPath, tt.keyPath, tt.caPath)
			require.Error(t, err)
			assert.Nil(t, cfg, "returned config should be nil on error")
			assert.ErrorContains(t, err, tt.wantErrText)
		})
	}
}

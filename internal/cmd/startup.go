package cmd

// startup.go — sub-functions extracted from run() to keep the top-level startup
// function readable. Each function handles a single, cohesive concern.

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	"github.com/github/gh-aw-mcpg/internal/httputil"
	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/server"
)

// setupTLSListener creates a net.Listener on listenAddr, optionally wrapping it
// with TLS when certPath and keyPath are both non-empty. It enforces the co-requirement
// between cert and key (both or neither), and the requirement that a CA path implies
// both cert and key are set (mutual TLS).
//
// Returns the listener, a boolean indicating whether TLS is enabled, and any error.
// On error, any partially opened TCP listener is closed before returning.
func setupTLSListener(listenAddr, certPath, keyPath, caPath string) (net.Listener, bool, error) {
	hasCert := certPath != ""
	hasKey := keyPath != ""
	hasCA := caPath != ""
	debugLog.Printf("TLS configuration: hasCert=%v, hasKey=%v, hasCA=%v", hasCert, hasKey, hasCA)
	if hasCert != hasKey {
		return nil, false, fmt.Errorf("--tls-cert and --tls-key must both be provided together")
	}
	if hasCA && !hasCert {
		return nil, false, fmt.Errorf("--tls-ca requires --tls-cert and --tls-key to also be set")
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, false, fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}
	debugLog.Printf("TCP listener created on %s", listenAddr)

	tlsEnabled := hasCert && hasKey
	if tlsEnabled {
		tlsCfg, err := httputil.LoadGatewayTLS(certPath, keyPath, caPath)
		if err != nil {
			_ = listener.Close()
			return nil, false, fmt.Errorf("failed to configure TLS: %w", err)
		}
		listener = tls.NewListener(listener, tlsCfg)
		mtlsNote := ""
		if caPath != "" {
			mtlsNote = " (mTLS enabled)"
		}
		logger.StartupInfo("TLS enabled: cert=%s, key=%s, ca=%s — listening on https://%s%s",
			certPath, keyPath, caPath, listenAddr, mtlsNote)
	} else {
		logger.StartupInfo("TLS not configured — listening on http://%s (set --tls-cert/--tls-key to enable)", listenAddr)
	}

	return listener, tlsEnabled, nil
}

// buildHTTPServer creates the *http.Server for the given routing mode, sets its
// BaseContext so all requests inherit the startup context (which carries the W3C
// parent trace span), and wires the HTTP shutdown and exit hooks into the unified
// server so the /close handler can drain requests and cancel the context cleanly.
func buildHTTPServer(
	ctx context.Context,
	mode, listenAddr string,
	unifiedServer *server.UnifiedServer,
	agentIDs []string, hmacSecret string,
	cancel context.CancelFunc,
) *http.Server {
	debugLog.Printf("Building HTTP server: mode=%s, listenAddr=%s, authEnabled=%v, hmacEnabled=%v",
		mode, listenAddr, len(agentIDs) > 0, hmacSecret != "")
	var httpServer *http.Server
	if mode == "routed" {
		logger.StartupInfo("Starting MCPG in ROUTED mode on %s", listenAddr)
		logger.StartupInfo("Routes: /mcp/<server> where <server> is one of: %v", unifiedServer.GetServerIDs())
		httpServer = server.CreateHTTPServerForRoutedMode(listenAddr, unifiedServer, agentIDs, hmacSecret)
	} else {
		logger.StartupInfo("Starting MCPG in UNIFIED mode on %s", listenAddr)
		logger.StartupInfo("Endpoint: /mcp")
		httpServer = server.CreateHTTPServerForMCP(listenAddr, unifiedServer, agentIDs, hmacSecret)
	}

	// Set BaseContext so every incoming request inherits the startup context,
	// which carries the configured W3C parent span context (traceId/spanId).
	// This ensures HTTP handler spans join the workflow trace even when the
	// calling client does not send traceparent headers.
	httpServer.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}

	// Register the HTTP server shutdown function so the /close handler can drain
	// in-flight requests before exiting (spec 5.1.3).
	unifiedServer.SetHTTPShutdown(httpServer.Shutdown)
	debugLog.Print("Registered HTTP shutdown hook on unified server")

	// Register exit function so /close handler cancels context instead of calling
	// os.Exit(0). This allows deferred cleanup (TracerProvider.Shutdown) to flush
	// buffered spans before the process terminates.
	unifiedServer.SetExitFunc(cancel)
	debugLog.Print("Registered exit hook (context cancel) on unified server")

	return httpServer
}

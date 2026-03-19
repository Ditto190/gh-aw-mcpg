package cmd

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/github/gh-aw-mcpg/internal/logger"
	"github.com/github/gh-aw-mcpg/internal/proxy"
	"github.com/spf13/cobra"
)

// Proxy subcommand flag variables
var (
	proxyGuardWasm string
	proxyPolicy    string
	proxyToken     string
	proxyListen    string
	proxyLogDir    string
	proxyDIFCMode  string
	proxyAPIURL    string
)

func init() {
	rootCmd.AddCommand(newProxyCmd())
}

func newProxyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Run as a GitHub API filtering proxy",
		Long: `Run the gateway in proxy mode — an HTTP forward proxy that intercepts
gh CLI requests and applies DIFC filtering using the same guard WASM module.

Usage with the gh CLI:

  # Start the proxy
  awmg proxy \
    --guard-wasm guards/github-guard/github_guard.wasm \
    --policy '{"allow-only":{"repos":["org/repo"],"min-integrity":"approved"}}' \
    --github-token "$GITHUB_TOKEN" \
    --listen localhost:8080

  # Point gh at the proxy
  GH_HOST=localhost:8080 GH_TOKEN="$GITHUB_TOKEN" gh issue list -R org/repo`,
		SilenceUsage: true,
		RunE:         runProxy,
	}

	cmd.Flags().StringVar(&proxyGuardWasm, "guard-wasm", "", "Path to the guard WASM module (required)")
	cmd.Flags().StringVar(&proxyPolicy, "policy", getDefaultGuardPolicyJSON(), "Guard policy JSON")
	cmd.Flags().StringVar(&proxyToken, "github-token", os.Getenv("GITHUB_TOKEN"), "GitHub API token")
	cmd.Flags().StringVarP(&proxyListen, "listen", "l", "127.0.0.1:8080", "HTTP proxy listen address")
	cmd.Flags().StringVar(&proxyLogDir, "log-dir", getDefaultLogDir(), "Log file directory")
	cmd.Flags().StringVar(&proxyDIFCMode, "guards-mode", "filter", "DIFC enforcement mode: strict, filter, propagate")
	cmd.Flags().StringVar(&proxyAPIURL, "github-api-url", proxy.DefaultGitHubAPIBase, "Upstream GitHub API URL")

	cmd.MarkFlagRequired("guard-wasm")

	return cmd
}

func runProxy(cmd *cobra.Command, args []string) error {
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize loggers
	if err := logger.InitFileLogger(proxyLogDir, "proxy.log"); err != nil {
		log.Printf("Warning: Failed to initialize file logger: %v", err)
	}
	if err := logger.InitJSONLLogger(proxyLogDir, "proxy-rpc.jsonl"); err != nil {
		log.Printf("Warning: Failed to initialize JSONL logger: %v", err)
	}

	logger.LogInfo("startup", "MCPG Proxy starting: listen=%s, guard=%s, mode=%s", proxyListen, proxyGuardWasm, proxyDIFCMode)

	// Resolve GitHub token
	token := proxyToken
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	}

	// Create the proxy server
	proxySrv, err := proxy.New(ctx, proxy.Config{
		WasmPath:     proxyGuardWasm,
		Policy:       proxyPolicy,
		GitHubToken:  token,
		GitHubAPIURL: proxyAPIURL,
		DIFCMode:     proxyDIFCMode,
	})
	if err != nil {
		return fmt.Errorf("failed to create proxy server: %w", err)
	}

	// Create and start the HTTP server
	httpServer := &http.Server{
		Addr:    proxyListen,
		Handler: proxySrv.Handler(),
	}

	// Start HTTP server in background
	go func() {
		listener, err := net.Listen("tcp", proxyListen)
		if err != nil {
			log.Printf("Failed to listen on %s: %v", proxyListen, err)
			cancel()
			return
		}

		actualAddr := listener.Addr().String()
		log.Printf("MCPG Proxy listening on %s", actualAddr)
		logger.LogInfo("startup", "Proxy listening on %s", actualAddr)

		// Print connection info
		fmt.Fprintf(os.Stderr, "\nMCPG GitHub API Proxy\n")
		fmt.Fprintf(os.Stderr, "  Listening: %s\n", actualAddr)
		fmt.Fprintf(os.Stderr, "  Mode:      %s\n", proxyDIFCMode)
		fmt.Fprintf(os.Stderr, "  Guard:     %s\n", proxyGuardWasm)
		fmt.Fprintf(os.Stderr, "\nConnect with:\n")
		fmt.Fprintf(os.Stderr, "  GH_HOST=%s GH_TOKEN=<token> gh ...\n\n", actualAddr)

		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("Shutting down proxy...")
	logger.LogInfo("shutdown", "Proxy shutting down")

	return httpServer.Close()
}

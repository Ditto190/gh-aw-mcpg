package cmd

// DIFC (Decentralized Information Flow Control) related flags

import (
	"fmt"
	"os"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/envutil"
	"github.com/spf13/cobra"
)

// DIFC flag defaults
const (
	defaultEnableDIFC       = false
	defaultDIFCMode         = "strict" // strict, filter, or propagate
	defaultConfigExtensions = false
)

// Valid DIFC enforcement modes
var validDIFCModes = map[string]bool{
	"strict":    true,
	"filter":    true,
	"propagate": true,
}

// DIFC flag variables
var (
	enableDIFC       bool
	difcMode         string
	enableConfigExt  bool   // Enable config extensions (guards, session labels)
	sessionSecrecy   string // Comma-separated initial secrecy labels
	sessionIntegrity string // Comma-separated initial integrity labels
)

func init() {
	RegisterFlag(func(cmd *cobra.Command) {
		cmd.Flags().BoolVar(&enableDIFC, "enable-difc", getDefaultEnableDIFC(), "Enable DIFC enforcement for information flow control")
		cmd.Flags().MarkHidden("enable-difc")
		cmd.Flags().StringVar(&difcMode, "difc-mode", getDefaultDIFCMode(), "DIFC enforcement mode: strict (deny violations), filter (remove denied tools), or propagate (auto-adjust agent labels on reads)")
		cmd.Flags().BoolVar(&enableConfigExt, "enable-config-extensions", getDefaultConfigExtensions(), "Enable config extensions (guards, session labels) - required for DIFC session label features")
		cmd.Flags().StringVar(&sessionSecrecy, "session-secrecy", getDefaultSessionSecrecy(), "Comma-separated initial secrecy labels for agent sessions (requires --enable-config-extensions)")
		cmd.Flags().StringVar(&sessionIntegrity, "session-integrity", getDefaultSessionIntegrity(), "Comma-separated initial integrity labels for agent sessions (requires --enable-config-extensions)")
	})
}

// getDefaultEnableDIFC returns the default DIFC setting, checking MCP_GATEWAY_ENABLE_DIFC
// environment variable first, then falling back to the hardcoded default (false)
func getDefaultEnableDIFC() bool {
	return envutil.GetEnvBool("MCP_GATEWAY_ENABLE_DIFC", defaultEnableDIFC)
}

// getDefaultDIFCMode returns the default DIFC mode, checking MCP_GATEWAY_DIFC_MODE
// environment variable first, then falling back to the hardcoded default (strict)
func getDefaultDIFCMode() string {
	if envMode := os.Getenv("MCP_GATEWAY_DIFC_MODE"); envMode != "" {
		mode := strings.ToLower(envMode)
		if validDIFCModes[mode] {
			return mode
		}
	}
	return defaultDIFCMode
}

// getDefaultConfigExtensions returns the default config extensions setting,
// checking MCP_GATEWAY_CONFIG_EXTENSIONS environment variable first
func getDefaultConfigExtensions() bool {
	return envutil.GetEnvBool("MCP_GATEWAY_CONFIG_EXTENSIONS", defaultConfigExtensions)
}

// getDefaultSessionSecrecy returns the default session secrecy labels from
// MCP_GATEWAY_SESSION_SECRECY environment variable
func getDefaultSessionSecrecy() string {
	return os.Getenv("MCP_GATEWAY_SESSION_SECRECY")
}

// getDefaultSessionIntegrity returns the default session integrity labels from
// MCP_GATEWAY_SESSION_INTEGRITY environment variable
func getDefaultSessionIntegrity() string {
	return os.Getenv("MCP_GATEWAY_SESSION_INTEGRITY")
}

// ValidateDIFCMode validates the DIFC mode flag value and returns an error if invalid
func ValidateDIFCMode(mode string) error {
	if !validDIFCModes[strings.ToLower(mode)] {
		return fmt.Errorf("invalid DIFC mode %q: must be one of: strict, filter, propagate", mode)
	}
	return nil
}

// parseSessionLabels parses a comma-separated string of labels into a slice
func parseSessionLabels(labels string) []string {
	if labels == "" {
		return nil
	}
	parts := strings.Split(labels, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

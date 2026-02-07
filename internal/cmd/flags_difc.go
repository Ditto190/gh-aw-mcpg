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
	defaultEnableDIFC = false
	defaultDIFCMode   = "strict" // strict, filter, or propagate
)

// Valid DIFC enforcement modes
var validDIFCModes = map[string]bool{
	"strict":    true,
	"filter":    true,
	"propagate": true,
}

// DIFC flag variables
var (
	enableDIFC bool
	difcMode   string
)

func init() {
	RegisterFlag(func(cmd *cobra.Command) {
		cmd.Flags().BoolVar(&enableDIFC, "enable-difc", getDefaultEnableDIFC(), "Enable DIFC enforcement for information flow control")
		cmd.Flags().MarkHidden("enable-difc")
		cmd.Flags().StringVar(&difcMode, "difc-mode", getDefaultDIFCMode(), "DIFC enforcement mode: strict (deny violations), filter (remove denied tools), or propagate (auto-adjust agent labels on reads)")
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

// ValidateDIFCMode validates the DIFC mode flag value and returns an error if invalid
func ValidateDIFCMode(mode string) error {
	if !validDIFCModes[strings.ToLower(mode)] {
		return fmt.Errorf("invalid DIFC mode %q: must be one of: strict, filter, propagate", mode)
	}
	return nil
}

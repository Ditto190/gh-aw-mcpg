package config

import (
	"fmt"
	"strconv"

	"github.com/github/gh-aw-mcpg/internal/config/rules"
	"github.com/github/gh-aw-mcpg/internal/envutil"
)

// GetGatewayPortFromEnv returns the MCP_GATEWAY_PORT value, parsed as int
func GetGatewayPortFromEnv() (int, error) {
	portStr := envutil.GetEnvString("MCP_GATEWAY_PORT", "")
	if portStr == "" {
		logConfig.Print("MCP_GATEWAY_PORT not set in environment")
		return 0, fmt.Errorf("MCP_GATEWAY_PORT environment variable not set")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		logConfig.Printf("MCP_GATEWAY_PORT=%q is not a valid integer: %v", portStr, err)
		return 0, fmt.Errorf("invalid MCP_GATEWAY_PORT value: %s", portStr)
	}

	if validationErr := rules.PortRange(port, "MCP_GATEWAY_PORT"); validationErr != nil {
		logConfig.Printf("MCP_GATEWAY_PORT=%d is outside valid port range: %s", port, validationErr.Message)
		return 0, fmt.Errorf("%s", validationErr.Message)
	}

	logConfig.Printf("MCP_GATEWAY_PORT resolved to %d", port)
	return port, nil
}

// GetGatewayDomainFromEnv returns the MCP_GATEWAY_DOMAIN value
func GetGatewayDomainFromEnv() string {
	domain := envutil.GetEnvString("MCP_GATEWAY_DOMAIN", "")
	if domain != "" {
		logConfig.Printf("MCP_GATEWAY_DOMAIN=%q", domain)
	}
	return domain
}

// GetGatewayAPIKeyFromEnv returns the MCP_GATEWAY_API_KEY value
func GetGatewayAPIKeyFromEnv() string {
	key := envutil.GetEnvString("MCP_GATEWAY_API_KEY", "")
	if key != "" {
		logConfig.Print("MCP_GATEWAY_API_KEY found in environment")
	} else {
		logConfig.Print("MCP_GATEWAY_API_KEY not set in environment")
	}
	return key
}

package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/util"
)

func validateGatewayConfig(gateway *StdinGatewayConfig) error {
	return validateGatewayConfigWithAgentRequirement(gateway, true)
}

// validateGatewayConfigForPatterns validates rule-based string patterns without enforcing
// the global exactly-one-agent-selection requirement. This is used by validateRuleBasedPatterns
// to keep pattern validation focused on the field being checked.
func validateGatewayConfigForPatterns(gateway *StdinGatewayConfig) error {
	return validateGatewayConfigWithAgentRequirement(gateway, false)
}

// validateGatewayConfigWithAgentRequirement centralizes gateway validation and optionally
// enforces the 1.17.0 requirement that exactly one agent ID selection be configured.
func validateGatewayConfigWithAgentRequirement(gateway *StdinGatewayConfig, requireAgentSelection bool) error {
	if gateway == nil {
		logValidation.Print("No gateway config to validate")
		return nil
	}

	logValidation.Print("Validating gateway configuration")

	if gateway.agentIDSet && strings.TrimSpace(gateway.AgentID) == "" {
		return fmt.Errorf("gateway.agentId must be a non-empty string when provided")
	}
	if gateway.legacyAPIKeySet && strings.TrimSpace(gateway.APIKey) == "" {
		return fmt.Errorf("gateway.apiKey must be a non-empty string when provided")
	}
	if requireAgentSelection && !gateway.agentIDSet && !gateway.legacyAPIKeySet && !gateway.agentIDsSet && gateway.AgentID == "" && gateway.APIKey == "" && len(gateway.AgentIDs) == 0 {
		return fmt.Errorf("gateway.agentId or gateway.agentIds must be configured; exactly one selection is required")
	}

	if err := validateAgentIDs(gateway.AgentIDs, gateway.agentIDsSet || gateway.AgentIDs != nil, "agentIds"); err != nil {
		return err
	}
	if (gateway.agentIDSet || gateway.AgentID != "") && (gateway.agentIDsSet || gateway.AgentIDs != nil) {
		return fmt.Errorf("gateway.agentId cannot be combined with gateway.agentIds")
	}
	if (gateway.legacyAPIKeySet || gateway.APIKey != "") && (gateway.agentIDsSet || gateway.AgentIDs != nil) {
		return fmt.Errorf("gateway.apiKey cannot be combined with gateway.agentIds")
	}

	// Validate port range using centralized rules
	if err := validateOptionalInt(gateway.Port, "Validating gateway port",
		func(v int) *ValidationError { return PortRange(v, "gateway.port") }); err != nil {
		return err
	}

	// Validate timeout values using centralized rules
	if err := validateOptionalInt(gateway.StartupTimeout, "Validating startup timeout",
		func(v int) *ValidationError { return TimeoutPositive(v, "startupTimeout", "gateway.startupTimeout") }); err != nil {
		return err
	}

	if err := validateOptionalInt(gateway.ToolTimeout, "Validating tool timeout",
		func(v int) *ValidationError {
			return TimeoutMinimum(v, ToolTimeoutMin, "toolTimeout", "gateway.toolTimeout")
		}); err != nil {
		return err
	}

	if err := validateContainerRuntimeValue(gateway.ContainerRuntime, "gateway.containerRuntime"); err != nil {
		return err
	}
	if gateway.Dockerless {
		runtime := normalizeContainerRuntime(gateway.ContainerRuntime)
		if runtime != "" && runtime != containerRuntimePodman {
			return fmt.Errorf("gateway.containerRuntime must be %q when gateway.dockerless is enabled", containerRuntimePodman)
		}
	}
	if err := validateContainerRuntimeCommandNotBlank(
		gateway.ContainerRuntimeCommand,
		"containerRuntimeCommand",
		"gateway.containerRuntimeCommand",
	); err != nil {
		return err
	}
	if gateway.Dockerless && gateway.ContainerRuntimeCommand != "" &&
		filepath.Base(strings.TrimSpace(gateway.ContainerRuntimeCommand)) != containerRuntimePodman {
		return fmt.Errorf("gateway.containerRuntimeCommand must resolve to %q when gateway.dockerless is enabled", containerRuntimePodman)
	}

	// Validate payloadDir if provided (per schema: must be absolute path)
	if gateway.PayloadDir != "" {
		logValidation.Printf("Validating payload directory: %s", gateway.PayloadDir)
		if err := AbsolutePath(gateway.PayloadDir, "payloadDir", "gateway.payloadDir"); err != nil {
			return err
		}
	}

	// Validate payloadSizeThreshold per spec §4.1.3.3: must be a positive integer when present.
	if err := validateOptionalInt(gateway.PayloadSizeThreshold, "Validating payload size threshold",
		func(v int) *ValidationError {
			return PositiveInteger(v, "payloadSizeThreshold", "gateway.payloadSizeThreshold")
		}); err != nil {
		return err
	}

	// Validate trustedBots per spec §4.1.3.4: must be non-empty array when present
	if err := validateTrustedBots(gateway.TrustedBots); err != nil {
		return err
	}

	// Validate OpenTelemetry config per spec §4.1.3.6 when present
	if gateway.OpenTelemetry != nil {
		tracingCfg := &TracingConfig{
			Endpoint:    gateway.OpenTelemetry.Endpoint,
			TraceID:     gateway.OpenTelemetry.TraceID,
			SpanID:      gateway.OpenTelemetry.SpanID,
			ServiceName: gateway.OpenTelemetry.ServiceName,
		}
		if err := validateOpenTelemetryConfig(tracingCfg, true); err != nil {
			return err
		}
	}

	logValidation.Print("Gateway config validation passed")
	return nil
}

func validateAgentIDs(agentIDs []string, defined bool, fieldName string) error {
	if err := validateNonEmptyStringSlice(agentIDs, defined, fieldName, ""); err != nil {
		return err
	}
	// Reject duplicate agent IDs: each identity must be unique so per-agent policy,
	// session, and DIFC state can be attributed deterministically.
	seen := make(map[string]struct{}, len(agentIDs))
	for _, id := range agentIDs {
		if _, dup := seen[id]; dup {
			return fmt.Errorf("gateway.%s must not contain duplicate agent ID %q", fieldName, util.HashIdentifierForLog(id))
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateGatewayPayloadSizeThreshold(value int, fieldName, jsonPath string) error {
	if ve := PositiveInteger(value, fieldName, jsonPath); ve != nil {
		return ve
	}
	return nil
}

func validateContainerRuntimeCommandNotBlank(command, fieldName, jsonPath string) error {
	if len(strings.TrimSpace(command)) == 0 && command != "" {
		return InvalidValue(fieldName, fmt.Sprintf("%s cannot be empty or whitespace only", fieldName),
			jsonPath, "Provide a non-empty runtime command path/name or remove the field")
	}
	return nil
}

// validateTrustedBots checks that the trusted_bots/trustedBots list conforms to spec §4.1.3.4:
// when present, it must be a non-empty array of non-empty strings.
func validateTrustedBots(bots []string) error {
	return validateNonEmptyStringSlice(bots, bots != nil, "trusted_bots", " (spec §4.1.3.4)")
}

// validateTOMLStdioContainerization validates that TOML stdio servers use the selected container runtime command.
// This enforces MCP Gateway Specification Section 3.2.1: "Stdio-based MCP servers MUST be containerized."
func validateTOMLStdioContainerization(servers map[string]*ServerConfig, gateway *GatewayConfig) error {
	logValidation.Print("Validating TOML stdio server containerization requirement")
	expectedCommand := configuredContainerRuntimeCommand(gateway)

	for name, cfg := range servers {
		// Only validate stdio servers (or empty type which defaults to stdio)
		if IsStdioServerType(cfg.Type) {
			logValidation.Printf("Checking stdio server: name=%s, command=%s", name, cfg.Command)

			if cfg.Command != expectedCommand {
				logValidation.Printf("Validation failed: %s, name=%s, type=%s", fmt.Sprintf("stdio server using unexpected runtime command, command=%s, expected=%s", cfg.Command, expectedCommand), name, "stdio")
				return fmt.Errorf(
					"server '%s': stdio servers must use containerized execution (command must be '%s', got '%s'). "+
						"This is required by MCP Gateway Specification Section 3.2.1 (Containerization Requirement). "+
						"See: https://github.com/github/gh-aw/blob/main/docs/src/content/docs/reference/mcp-gateway.md#321-containerization-requirement",
					name, expectedCommand, cfg.Command)
			}
		}
	}

	logValidation.Print("TOML stdio containerization validation passed")
	return nil
}

// validateGuardPolicies validates all per-server guard policies in the config.
// It iterates over cfg.Guards and calls ValidateGuardPolicy for each non-nil policy.
func validateGuardPolicies(cfg *Config) error {
	logValidation.Printf("Validating guard policies: count=%d", len(cfg.Guards))
	for name, guardCfg := range cfg.Guards {
		if guardCfg != nil && guardCfg.Policy != nil {
			if err := ValidateGuardPolicy(guardCfg.Policy); err != nil {
				return fmt.Errorf("invalid policy for guard '%s': %w", name, err)
			}
		}
	}
	return nil
}

// validateRuleBasedPatterns validates additional rule-based string constraints that
// are not handled by schema validation alone.
func validateRuleBasedPatterns(stdinCfg *StdinConfig) error {
	logValidation.Printf("Validating string patterns: server_count=%d", len(stdinCfg.MCPServers))

	for name, server := range stdinCfg.MCPServers {
		jsonPath := fmt.Sprintf("mcpServers.%s", name)
		logValidation.Printf("Validating server: name=%s, type=%s", name, server.Type)

		if IsStdioServerType(server.Type) {
			if server.Container != "" && !containerPattern.MatchString(server.Container) {
				return InvalidPattern("container", server.Container,
					fmt.Sprintf("%s.container", jsonPath),
					"Use a valid container image format (e.g., 'ghcr.io/owner/image:tag', 'owner/image:latest', or 'ghcr.io/owner/image:tag@sha256:<digest>')")
			}

			if server.Entrypoint != "" && len(strings.TrimSpace(server.Entrypoint)) == 0 {
				return InvalidValue("entrypoint", "entrypoint cannot be empty or whitespace only",
					fmt.Sprintf("%s.entrypoint", jsonPath),
					"Provide a valid entrypoint path or remove the field")
			}
		}

		if server.Type == "http" {
			if server.URL != "" && !urlPattern.MatchString(server.URL) {
				return InvalidPattern("url", server.URL,
					fmt.Sprintf("%s.url", jsonPath),
					"Use a valid HTTP or HTTPS URL (e.g., 'https://api.example.com/mcp')")
			}
		}
	}

	if stdinCfg.Gateway != nil {
		if err := validateGatewayConfigForPatterns(stdinCfg.Gateway); err != nil {
			return err
		}

		if stdinCfg.Gateway.Domain != "" {
			domain := stdinCfg.Gateway.Domain
			if domain != "localhost" && domain != "host.docker.internal" &&
				!domainVarPattern.MatchString(domain) && !domainHostnamePattern.MatchString(domain) {
				return InvalidValue("domain",
					fmt.Sprintf("domain '%s' must be 'localhost', 'host.docker.internal', an RFC-1123 hostname label (e.g. 'awmg-mcpg'), or a variable expression", domain),
					"gateway.domain",
					"Use 'localhost', 'host.docker.internal', a topology hostname like 'awmg-mcpg', or a variable like '${MCP_GATEWAY_DOMAIN}'")
			}
		}
	}

	return nil
}

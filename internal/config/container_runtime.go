package config

import (
	"fmt"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/envutil"
)

const (
	containerRuntimeDocker = "docker"
	containerRuntimePodman = "podman"
)

func isNonEmptyWhitespace(s string) bool {
	return strings.TrimSpace(s) == "" && s != ""
}

func normalizeContainerRuntime(runtime string) string {
	return strings.ToLower(strings.TrimSpace(runtime))
}

func runtimeCommandForName(runtime string) string {
	if normalizeContainerRuntime(runtime) == containerRuntimePodman {
		return containerRuntimePodman
	}
	return containerRuntimeDocker
}

func validateContainerRuntimeValue(value, fieldName string) error {
	if isNonEmptyWhitespace(value) {
		return fmt.Errorf("%s must not be empty or whitespace-only when set", fieldName)
	}
	normalized := normalizeContainerRuntime(value)
	switch normalized {
	case "", containerRuntimeDocker, containerRuntimePodman:
		return nil
	default:
		logConfig.Printf("validateContainerRuntimeValue: unsupported runtime %q for field %s", value, fieldName)
		return fmt.Errorf("%s must be one of: docker, podman (got: %q)", fieldName, value)
	}
}

func configuredContainerRuntimeName(configRuntime string) string {
	runtime := normalizeContainerRuntime(configRuntime)
	if runtime == "" {
		logConfig.Printf("configuredContainerRuntimeName: no runtime configured, using default=%s", DefaultContainerRuntime)
		runtime = DefaultContainerRuntime
	}
	return runtime
}

func effectiveContainerRuntimeName(configRuntime string) string {
	runtime := configuredContainerRuntimeName(configRuntime)
	logConfig.Printf("effectiveContainerRuntimeName: configured runtime=%s", runtime)

	// Env override takes precedence over config.
	if envRuntimeRaw := envutil.GetEnvString("MCP_GATEWAY_CONTAINER_RUNTIME", ""); envRuntimeRaw != "" {
		if isNonEmptyWhitespace(envRuntimeRaw) {
			logConfig.Printf("Ignoring whitespace-only MCP_GATEWAY_CONTAINER_RUNTIME value")
		} else if err := validateContainerRuntimeValue(envRuntimeRaw, "MCP_GATEWAY_CONTAINER_RUNTIME"); err != nil {
			logConfig.Printf("Ignoring invalid MCP_GATEWAY_CONTAINER_RUNTIME=%q: %v", envRuntimeRaw, err)
		} else {
			logConfig.Printf("effectiveContainerRuntimeName: env override MCP_GATEWAY_CONTAINER_RUNTIME=%s", normalizeContainerRuntime(envRuntimeRaw))
			runtime = normalizeContainerRuntime(envRuntimeRaw)
		}
	}
	return runtime
}

func effectiveContainerRuntimeCommand(gateway *GatewayConfig) string {
	runtime := DefaultContainerRuntime
	if gateway != nil {
		runtime = gateway.ContainerRuntime
	}
	command := runtimeCommandForName(effectiveContainerRuntimeName(runtime))
	if gateway != nil {
		trimmedRuntimeCommand := strings.TrimSpace(gateway.ContainerRuntimeCommand)
		if trimmedRuntimeCommand != "" {
			logConfig.Printf("effectiveContainerRuntimeCommand: using custom runtime command=%s", trimmedRuntimeCommand)
			command = trimmedRuntimeCommand
		}
	}
	logConfig.Printf("effectiveContainerRuntimeCommand: resolved command=%s", command)
	return command
}

func configuredContainerRuntimeCommand(gateway *GatewayConfig) string {
	command := runtimeCommandForName(DefaultContainerRuntime)
	if gateway != nil {
		command = runtimeCommandForName(configuredContainerRuntimeName(gateway.ContainerRuntime))
	}
	if gateway != nil && strings.TrimSpace(gateway.ContainerRuntimeCommand) != "" {
		command = strings.TrimSpace(gateway.ContainerRuntimeCommand)
	}
	return command
}

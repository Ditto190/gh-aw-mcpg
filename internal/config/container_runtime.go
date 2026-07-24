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
	normalized := normalizeContainerRuntime(value)
	if strings.TrimSpace(value) == "" && value != "" {
		return fmt.Errorf("%s must not be empty or whitespace-only when set", fieldName)
	}
	switch normalized {
	case "", containerRuntimeDocker, containerRuntimePodman:
		return nil
	default:
		return fmt.Errorf("%s must be one of: docker, podman (got: %q)", fieldName, value)
	}
}

func configuredContainerRuntimeName(configRuntime string) string {
	runtime := normalizeContainerRuntime(configRuntime)
	if runtime == "" {
		runtime = DefaultContainerRuntime
	}
	return runtime
}

func effectiveContainerRuntimeName(configRuntime string) string {
	runtime := configuredContainerRuntimeName(configRuntime)

	// Env override takes precedence over config.
	if envRuntimeRaw := envutil.GetEnvString("MCP_GATEWAY_CONTAINER_RUNTIME", ""); envRuntimeRaw != "" {
		if err := validateContainerRuntimeValue(envRuntimeRaw, "MCP_GATEWAY_CONTAINER_RUNTIME"); err != nil {
			logConfig.Printf("Ignoring invalid MCP_GATEWAY_CONTAINER_RUNTIME=%q: %v", envRuntimeRaw, err)
		} else {
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
	if gateway != nil && strings.TrimSpace(gateway.ContainerRuntimeCommand) != "" {
		command = strings.TrimSpace(gateway.ContainerRuntimeCommand)
	}
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

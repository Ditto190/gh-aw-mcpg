package config

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// containerIDPattern validates that a container ID only contains valid characters (hex digits)
// Container IDs are 64 character hex strings, but short form (12 chars) is also valid
var containerIDPattern = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

// checkDockerAccessible verifies that the Docker daemon is accessible
func checkDockerAccessible() bool {
	// First check if the Docker socket exists
	socketPath := os.Getenv("DOCKER_HOST")
	if socketPath == "" {
		socketPath = "/var/run/docker.sock"
	} else {
		// Parse unix:// prefix if present
		socketPath = strings.TrimPrefix(socketPath, "unix://")
	}
	logEnv.Printf("Checking Docker socket accessibility: socketPath=%s", socketPath)

	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		logEnv.Printf("Docker socket not found: socketPath=%s", socketPath)
		return false
	}

	// Try to run docker info to verify connectivity
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	accessible := cmd.Run() == nil
	logEnv.Printf("Docker daemon check: accessible=%v", accessible)
	return accessible
}

// validateContainerID validates that the container ID is safe to use in commands
// Container IDs should only contain lowercase hex characters (a-f, 0-9)
func validateContainerID(containerID string) error {
	if containerID == "" {
		return fmt.Errorf("container ID is empty")
	}
	if !containerIDPattern.MatchString(containerID) {
		return fmt.Errorf("container ID contains invalid characters: must be 12-64 hex characters")
	}
	return nil
}

// runDockerInspect is a helper function that executes docker inspect with a given format template.
// It validates the container ID before running the command and returns the output as a string.
//
// Security Note: This is an internal helper function that should only be called with
// hardcoded format templates defined within this package. The formatTemplate parameter
// is not validated as it is never exposed to user input.
//
// Parameters:
//   - containerID: The Docker container ID to inspect (validated before use)
//   - formatTemplate: The Go template format string for docker inspect (e.g., "{{.Config.OpenStdin}}")
//
// Returns:
//   - output: The trimmed output from docker inspect
//   - error: Any validation or command execution error
func runDockerInspect(containerID, formatTemplate string) (string, error) {
	if err := validateContainerID(containerID); err != nil {
		return "", err
	}

	cmd := exec.Command("docker", "inspect", "--format", formatTemplate, containerID)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// checkPortMapping uses docker inspect to verify that the specified port is mapped
func checkPortMapping(containerID, port string) (bool, error) {
	output, err := runDockerInspect(containerID, "{{json .NetworkSettings.Ports}}")
	if err != nil {
		return false, err
	}

	// Parse the port from the output
	portKey := fmt.Sprintf("%s/tcp", port)

	// Check if the port is in the output with a host binding
	// The format is like: {"8000/tcp":[{"HostIp":"0.0.0.0","HostPort":"8000"}]}
	return strings.Contains(output, portKey) && strings.Contains(output, "HostPort"), nil
}

// checkStdinInteractive uses docker inspect to verify the container was started with -i flag
func checkStdinInteractive(containerID string) bool {
	output, err := runDockerInspect(containerID, "{{.Config.OpenStdin}}")
	if err != nil {
		return false
	}

	return output == "true"
}

// checkLogDirMounted uses docker inspect to verify the log directory is mounted
func checkLogDirMounted(containerID, logDir string) bool {
	output, err := runDockerInspect(containerID, "{{json .Mounts}}")
	if err != nil {
		return false
	}

	// Check if the log directory is in the mounts
	return strings.Contains(output, logDir)
}

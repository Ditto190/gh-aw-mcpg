package sys

import (
	"os"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/logger"
)

var logSys = logger.New("sys:container")

// IsRunningInContainer detects if the current process is running inside a container.
func IsRunningInContainer() bool {
	logSys.Print("Detecting container environment")

	// Method 1: Check for /.dockerenv file (Docker-specific)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		logSys.Print("Container detected via /.dockerenv")
		return true
	}

	// Method 2: Check /proc/1/cgroup for container indicators
	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil {
		content := string(data)
		if strings.Contains(content, "docker") ||
			strings.Contains(content, "containerd") ||
			strings.Contains(content, "kubepods") ||
			strings.Contains(content, "lxc") {
			logSys.Print("Container detected via /proc/1/cgroup")
			return true
		}
	}

	// Method 3: Check environment variable (set by Dockerfile)
	if os.Getenv("RUNNING_IN_CONTAINER") == "true" {
		logSys.Print("Container detected via RUNNING_IN_CONTAINER env var")
		return true
	}

	logSys.Print("No container indicators found, running on host")
	return false
}

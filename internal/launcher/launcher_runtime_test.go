package launcher

import (
	"testing"

	"github.com/github/gh-aw-mcpg/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestIsDirectStdioCommand(t *testing.T) {
	tests := []struct {
		name     string
		server   *config.ServerConfig
		expected bool
	}{
		{name: "docker runtime", server: &config.ServerConfig{Command: "docker", Args: []string{"run", "--rm"}}, expected: false},
		{name: "podman runtime", server: &config.ServerConfig{Command: "podman", Args: []string{"run", "--rm"}}, expected: false},
		{name: "podman runtime absolute path", server: &config.ServerConfig{Command: "/usr/local/bin/podman", Args: []string{"--events-backend=file", "run", "--rm"}}, expected: false},
		{name: "nerdctl runtime", server: &config.ServerConfig{Command: "nerdctl", Args: []string{"run", "--rm"}}, expected: false},
		{name: "custom runtime args start with run", server: &config.ServerConfig{Command: "/usr/local/bin/runtime", Args: []string{"run", "--rm"}}, expected: false},
		{name: "containerized metadata bypasses arg inference", server: &config.ServerConfig{Containerized: true, Command: "/usr/local/bin/runtime", Args: []string{"--namespace", "k8s.io"}}, expected: false},
		{name: "direct command", server: &config.ServerConfig{Command: "python", Args: []string{"-m", "server"}}, expected: true},
		{name: "direct node command", server: &config.ServerConfig{Command: "node", Args: []string{"server.js"}}, expected: true},
		{name: "direct shell script", server: &config.ServerConfig{Command: "/app/start.sh", Args: []string{}}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isDirectStdioCommand(tt.server))
		})
	}
}

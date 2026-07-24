package launcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDirectStdioCommand(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		args     []string
		expected bool
	}{
		{name: "docker runtime", command: "docker", args: []string{"run", "--rm"}, expected: false},
		{name: "podman runtime", command: "podman", args: []string{"run", "--rm"}, expected: false},
		{name: "podman runtime absolute path", command: "/usr/local/bin/podman", args: []string{"--events-backend=file", "run", "--rm"}, expected: false},
		{name: "nerdctl runtime", command: "nerdctl", args: []string{"run", "--rm"}, expected: false},
		{name: "custom runtime args start with run", command: "/usr/local/bin/runtime", args: []string{"run", "--rm"}, expected: false},
		{name: "direct command", command: "python", args: []string{"-m", "server"}, expected: true},
		{name: "direct node command", command: "node", args: []string{"server.js"}, expected: true},
		{name: "direct shell script", command: "/app/start.sh", args: []string{}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isDirectStdioCommand(tt.command, tt.args))
		})
	}
}

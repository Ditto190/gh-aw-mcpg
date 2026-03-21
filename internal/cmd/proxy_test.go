package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientAddr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "IPv4 wildcard becomes localhost",
			input:    "0.0.0.0:8080",
			expected: "localhost:8080",
		},
		{
			name:     "IPv6 wildcard :: becomes localhost",
			input:    "[::]:8443",
			expected: "localhost:8443",
		},
		{
			name:     "explicit localhost unchanged",
			input:    "localhost:3000",
			expected: "localhost:3000",
		},
		{
			name:     "explicit 127.0.0.1 unchanged",
			input:    "127.0.0.1:9090",
			expected: "127.0.0.1:9090",
		},
		{
			name:     "non-loopback host unchanged",
			input:    "192.168.1.1:8080",
			expected: "192.168.1.1:8080",
		},
		{
			name:     "invalid address returned as-is",
			input:    "not-an-addr",
			expected: "not-an-addr",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, clientAddr(tc.input))
		})
	}
}

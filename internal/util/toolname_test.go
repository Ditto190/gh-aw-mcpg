package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseServerIDFromToolName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		toolName string
		want     string
	}{
		{
			name:     "no separator returns full tool name",
			toolName: "list_repos",
			want:     "list_repos",
		},
		{
			name:     "normal prefixed tool name returns server ID",
			toolName: "github___list_repos",
			want:     "github",
		},
		{
			// strings.Cut("___list_repos", "___") → ("", "list_repos", true)
			// serverID=="" so the function falls into the !ok||serverID=="" branch
			// and returns the original toolName unchanged.
			name:     "tool name starting with separator returns full name",
			toolName: "___list_repos",
			want:     "___list_repos",
		},
		{
			name:     "empty tool name returns empty string",
			toolName: "",
			want:     "",
		},
		{
			// strings.Cut("___", "___") → ("", "", true); serverID=="" → returns "___"
			name:     "separator only returns full name",
			toolName: "___",
			want:     "___",
		},
		{
			// strings.Cut splits on the FIRST occurrence only.
			name:     "multiple separators returns portion before first",
			toolName: "github___owner___list_repos",
			want:     "github",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ParseServerIDFromToolName(tt.toolName)
			assert.Equal(t, tt.want, got)
		})
	}
}

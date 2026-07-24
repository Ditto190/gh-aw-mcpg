package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeStringCI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "already normalized", input: "public", want: "public"},
		{name: "trimmed and lowercased", input: "  Public  ", want: "public"},
		{name: "whitespace only", input: "   ", want: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, NormalizeStringCI(tt.input))
		})
	}
}

package strutil

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRandomHex(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		wantLen int
		wantErr bool
	}{
		{
			name:    "zero bytes produces empty string",
			n:       0,
			wantLen: 0,
		},
		{
			name:    "1 byte produces 2 hex chars",
			n:       1,
			wantLen: 2,
		},
		{
			name:    "16 bytes produces 32 hex chars",
			n:       16,
			wantLen: 32,
		},
		{
			name:    "32 bytes produces 64 hex chars",
			n:       32,
			wantLen: 64,
		},
		{
			name:    "negative n returns error",
			n:       -1,
			wantErr: true,
		},
		{
			name:    "large negative n returns error",
			n:       -100,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RandomHex(tt.n)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, result, "result should be empty on error")
				return
			}
			require.NoError(t, err)
			assert.Len(t, result, tt.wantLen)
		})
	}
}

// TestRandomHex_ErrorMessageContainsSize verifies the error for negative n includes the invalid value.
func TestRandomHex_ErrorMessageContainsSize(t *testing.T) {
	_, err := RandomHex(-5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-5", "error message should include the invalid size")
}

// TestRandomHex_IsValidHex verifies the output is a valid lowercase hex-encoded string
// and that decoding it yields exactly n bytes.
func TestRandomHex_IsValidHex(t *testing.T) {
	result, err := RandomHex(16)
	require.NoError(t, err)

	decoded, decodeErr := hex.DecodeString(result)
	require.NoError(t, decodeErr, "result should be valid hex-encoded string")
	assert.Len(t, decoded, 16, "decoded bytes should have length equal to input n")
}

func TestRandomHex_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := RandomHex(16)
		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.False(t, seen[id], "RandomHex should produce unique values")
		seen[id] = true
	}
}

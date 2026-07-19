package auth

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorReader is a test helper io.Reader that always returns the configured error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

// TestGenerateRandomAgentID_RandomFailure verifies that GenerateRandomAgentID
// correctly wraps and propagates errors from the underlying random source.
// This test must NOT run in parallel because it temporarily replaces the
// global crypto/rand.Reader.
func TestGenerateRandomAgentID_RandomFailure(t *testing.T) {
	syntheticErr := errors.New("synthetic entropy failure")

	origReader := rand.Reader
	rand.Reader = &errorReader{err: syntheticErr}
	defer func() { rand.Reader = origReader }()

	key, err := GenerateRandomAgentID()

	assert.Empty(t, key, "key should be empty when random generation fails")
	require.Error(t, err, "should return an error when the random source fails")
	assert.ErrorIs(t, err, syntheticErr, "error should wrap the underlying source error")
	assert.Contains(t, err.Error(), "failed to generate random agent ID",
		"error message should describe the failure context")
}

// TestGenerateRandomAgentID_RecoveryAfterFailure verifies that
// GenerateRandomAgentID works correctly after the random source is restored,
// confirming that no state is leaked between calls.
// This test must NOT run in parallel because it temporarily replaces the
// global crypto/rand.Reader.
func TestGenerateRandomAgentID_RecoveryAfterFailure(t *testing.T) {
	origReader := rand.Reader
	rand.Reader = &errorReader{err: errors.New("transient failure")}
	_, err := GenerateRandomAgentID()
	require.Error(t, err, "should fail with broken reader")

	// Restore and verify subsequent call succeeds.
	rand.Reader = origReader
	key, err := GenerateRandomAgentID()
	require.NoError(t, err, "should succeed after reader is restored")
	assert.Len(t, key, 64, "restored call should return 64-char hex key")
}

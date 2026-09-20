package delegation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateAgainstEnvelope exercises every branch of
// (*Store).validateAgainstEnvelope directly, including the invocation-id,
// schema-hash, and invocation-deadline checks that are not reachable through
// the higher-level CreateOrConfirm table in TestCreateOrConfirm_RejectsOutsideEnvelope,
// plus the defensive "envelope admits no schema hash" invariant guard that is
// unreachable via a Store constructed through NewStore (since NewStore's
// Envelope.Validate call already rejects that combination) and must be
// exercised by constructing a Store whose envelope bypasses Validate.
func TestValidateAgainstEnvelope(t *testing.T) {
	fixedNow := time.Now()

	t.Run("valid request passes", func(t *testing.T) {
		store, _ := newTestStore(t)
		require.NoError(t, store.validateAgainstEnvelope(validRequest(), fixedNow))
	})

	t.Run("envelope expired", func(t *testing.T) {
		store, envelope := newTestStore(t)
		envelope.ExpiresAt = fixedNow.Add(-time.Minute)
		store.envelope = envelope
		err := store.validateAgainstEnvelope(validRequest(), fixedNow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "envelope expired")
	})

	t.Run("missing invocation id", func(t *testing.T) {
		store, _ := newTestStore(t)
		req := validRequest()
		req.InvocationID = ""
		err := store.validateAgainstEnvelope(req, fixedNow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invocation id is required")
	})

	t.Run("missing schema hash", func(t *testing.T) {
		store, _ := newTestStore(t)
		req := validRequest()
		req.SchemaHash = ""
		err := store.validateAgainstEnvelope(req, fixedNow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "schema hash is required")
	})

	t.Run("schema hash outside allowed set", func(t *testing.T) {
		store, _ := newTestStore(t)
		req := validRequest()
		req.SchemaHash = "sha256:not-allowed"
		err := store.validateAgainstEnvelope(req, fixedNow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "schema hash outside envelope")
	})

	t.Run("dynamic schema hash mode admits any hash when under the bound", func(t *testing.T) {
		store, envelope := newTestStore(t)
		envelope.AllowedSchemaHashes = nil
		envelope.MaxDynamicSchemaHashes = 5
		store.envelope = envelope
		req := validRequest()
		req.SchemaHash = "sha256:dynamic-hash"
		require.NoError(t, store.validateAgainstEnvelope(req, fixedNow))
	})

	t.Run("defensive guard: envelope admits no schema hash at all", func(t *testing.T) {
		store, envelope := newTestStore(t)
		// Bypass Envelope.Validate (which would normally reject this
		// combination) to exercise the defensive fail-closed guard directly,
		// simulating an envelope constructed without going through Validate.
		envelope.AllowedSchemaHashes = nil
		envelope.MaxDynamicSchemaHashes = 0
		store.envelope = envelope
		err := store.validateAgainstEnvelope(validRequest(), fixedNow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "envelope does not admit any schema hash")
	})

	t.Run("invocation deadline already elapsed", func(t *testing.T) {
		store, _ := newTestStore(t)
		req := validRequest()
		req.InvocationExpiresAt = fixedNow.Add(-time.Second)
		err := store.validateAgainstEnvelope(req, fixedNow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invocation deadline has elapsed")
	})

	t.Run("invocation deadline exactly now is elapsed (not before)", func(t *testing.T) {
		store, _ := newTestStore(t)
		req := validRequest()
		req.InvocationExpiresAt = fixedNow
		err := store.validateAgainstEnvelope(req, fixedNow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invocation deadline has elapsed")
	})

	t.Run("invocation deadline in the future passes", func(t *testing.T) {
		store, _ := newTestStore(t)
		req := validRequest()
		req.InvocationExpiresAt = fixedNow.Add(time.Minute)
		require.NoError(t, store.validateAgainstEnvelope(req, fixedNow))
	})

	t.Run("zero invocation deadline is not checked", func(t *testing.T) {
		store, _ := newTestStore(t)
		req := validRequest()
		req.InvocationExpiresAt = time.Time{}
		require.NoError(t, store.validateAgainstEnvelope(req, fixedNow))
	})
}

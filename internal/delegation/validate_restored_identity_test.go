package delegation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// validIdentityForEnvelope returns an Identity that satisfies
// validateRestoredIdentity against the provided envelope at the given generation,
// unless individually mutated by each test case.
func validIdentityForEnvelope(envelope *Envelope, generation uint64) *Identity {
	req := validRequest()
	if envelope != nil {
		req.RunID = envelope.RunID
		req.EnclaveBackend = envelope.EnclaveBackend
		req.ToolPolicy = envelope.ToolPolicy
		if len(envelope.AllowedRepositories) > 0 {
			req.Repository = envelope.AllowedRepositories[0]
		}
		if len(envelope.AllowedSchemaHashes) > 0 {
			req.SchemaHash = envelope.AllowedSchemaHashes[0]
		}
	}

	now := time.Now()
	expiresAt := now.Add(time.Minute)
	if envelope != nil && expiresAt.After(envelope.ExpiresAt) {
		expiresAt = envelope.ExpiresAt
	}

	return &Identity{
		Handle:            "handle-1",
		ExecutorBearer:    "bearer-1",
		delegationBinding: bindingFromRequest(req),
		RequestedTTL:      req.RequestedTTL,
		ExpiresAt:         expiresAt,
		PolicyGeneration:  generation,
		IdempotencyKey:    req.IdempotencyKey,
		CreatedAt:         now,
	}
}

func TestValidateRestoredIdentity(t *testing.T) {
	t.Run("valid identity passes", func(t *testing.T) {
		envelope := validEnvelope()
		identity := validIdentityForEnvelope(envelope, 1)
		assert.NoError(t, validateRestoredIdentity(identity, envelope, 1))
	})

	t.Run("empty handle is rejected", func(t *testing.T) {
		envelope := validEnvelope()
		identity := validIdentityForEnvelope(envelope, 1)
		identity.Handle = ""
		err := validateRestoredIdentity(identity, envelope, 1)
		assert.ErrorContains(t, err, "invalid identity credential or generation")
	})

	t.Run("empty executor bearer is rejected", func(t *testing.T) {
		envelope := validEnvelope()
		identity := validIdentityForEnvelope(envelope, 1)
		identity.ExecutorBearer = ""
		err := validateRestoredIdentity(identity, envelope, 1)
		assert.ErrorContains(t, err, "invalid identity credential or generation")
	})

	t.Run("mismatched generation is rejected", func(t *testing.T) {
		envelope := validEnvelope()
		identity := validIdentityForEnvelope(envelope, 1)
		err := validateRestoredIdentity(identity, envelope, 2)
		assert.ErrorContains(t, err, "invalid identity credential or generation")
	})

	t.Run("identity expiry after envelope expiry is rejected", func(t *testing.T) {
		envelope := validEnvelope()
		identity := validIdentityForEnvelope(envelope, 1)
		identity.ExpiresAt = envelope.ExpiresAt.Add(time.Second)
		err := validateRestoredIdentity(identity, envelope, 1)
		assert.ErrorContains(t, err, "identity expiry exceeds envelope expiry")
	})

	t.Run("identity expiry after invocation expiry is rejected", func(t *testing.T) {
		envelope := validEnvelope()
		identity := validIdentityForEnvelope(envelope, 1)
		identity.InvocationExpiresAt = identity.CreatedAt.Add(30 * time.Second)
		identity.ExpiresAt = identity.InvocationExpiresAt.Add(time.Second)
		err := validateRestoredIdentity(identity, envelope, 1)
		assert.ErrorContains(t, err, "identity expiry exceeds invocation expiry")
	})

	t.Run("zero invocation expiry does not trigger invocation expiry check", func(t *testing.T) {
		envelope := validEnvelope()
		identity := validIdentityForEnvelope(envelope, 1)
		identity.InvocationExpiresAt = time.Time{}
		// ExpiresAt is well within envelope expiry, so should pass.
		assert.NoError(t, validateRestoredIdentity(identity, envelope, 1))
	})

	t.Run("identity expiry at invocation expiry boundary is allowed", func(t *testing.T) {
		envelope := validEnvelope()
		identity := validIdentityForEnvelope(envelope, 1)
		identity.InvocationExpiresAt = identity.ExpiresAt
		assert.NoError(t, validateRestoredIdentity(identity, envelope, 1))
	})

	t.Run("binding outside envelope is rejected via validateAgainstEnvelope", func(t *testing.T) {
		envelope := validEnvelope()
		identity := validIdentityForEnvelope(envelope, 1)
		identity.Repository = "github/not-allowed"
		err := validateRestoredIdentity(identity, envelope, 1)
		assert.ErrorContains(t, err, "repository outside envelope")
	})

	t.Run("envelope already expired is rejected via validateAgainstEnvelope", func(t *testing.T) {
		envelope := validEnvelope()
		envelope.ExpiresAt = time.Now().Add(-time.Hour)
		identity := validIdentityForEnvelope(envelope, 1)
		identity.ExpiresAt = envelope.ExpiresAt.Add(-time.Minute)
		err := validateRestoredIdentity(identity, envelope, 1)
		assert.ErrorContains(t, err, "envelope expired")
	})
}

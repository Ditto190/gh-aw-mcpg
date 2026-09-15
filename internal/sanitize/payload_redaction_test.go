package sanitize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldRedactPayload(t *testing.T) {
	assert := assert.New(t)

	previous := PayloadRedactionEnabled()
	t.Cleanup(func() {
		SetPayloadRedaction(previous)
		SetRawPayloadLogsAllowed(false)
	})

	SetPayloadRedaction(false)
	SetRawPayloadLogsAllowed(false)
	assert.False(ShouldRedactPayload(false), "non-enclave traffic keeps its existing logging behavior")
	assert.True(ShouldRedactPayload(true), "enclave-scoped traffic is redacted by default")

	SetPayloadRedaction(true)
	assert.True(ShouldRedactPayload(false), "process-wide mode redacts every session")

	SetRawPayloadLogsAllowed(true)
	assert.False(ShouldRedactPayload(true), "privileged opt-in restores raw payload logging")
	assert.True(RawPayloadLogsAllowed())
}

func TestRedactedPayloadRenderings(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	payload := []byte(`{"result":{"body":"SENTINEL-PRIVATE"}}`)

	text := RedactedPayloadText(payload)
	assert.NotContains(text, "SENTINEL-PRIVATE")
	assert.Contains(text, "bytes=38")
	assert.Contains(text, "digest=hmac:")

	encoded := RedactedPayloadJSON(payload)
	assert.NotContains(string(encoded), "SENTINEL-PRIVATE")

	var decoded map[string]any
	require.NoError(json.Unmarshal(encoded, &decoded))
	assert.Equal(true, decoded["redacted"])
	assert.Equal("enclave_payload_redacted", decoded["reason"])
	assert.Equal(float64(len(payload)), decoded["bytes"])

	// The digest is stable so identical payloads stay correlatable across lines.
	assert.Equal(PayloadDigest(payload), PayloadDigest(payload))
	assert.NotEqual(PayloadDigest(payload), PayloadDigest([]byte(`{"result":{}}`)))

	// The digest is keyed with a per-process secret, so an artifact reader cannot
	// recover a payload drawn from a small candidate set by hashing guesses.
	plain := sha256.Sum256(payload)
	assert.NotContains(text, hex.EncodeToString(plain[:])[:16])
}

func TestRedactErrorForLog(t *testing.T) {
	assert := assert.New(t)

	assert.Empty(RedactErrorForLog(nil))
	assert.Equal("timeout", RedactErrorForLog(context.DeadlineExceeded))
	assert.Equal("canceled", RedactErrorForLog(context.Canceled))

	category := RedactErrorForLog(errors.New("backend returned SENTINEL-PRIVATE"))
	assert.NotContains(category, "SENTINEL-PRIVATE")
	assert.Contains(category, "error hmac:")
}

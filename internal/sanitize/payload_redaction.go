package sanitize

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

// Payload redaction exists for enclave-scoped MCP traffic. An enclave agent is
// authorized to disclose only the bounded result its broker validated, but the
// gateway logs (rpc-messages.jsonl, mcp-gateway.log, the per-server logs, and
// gateway.md) are uploaded as workflow artifacts by a job running outside the
// enclave. Persisting raw MCP request arguments or response content there
// republishes private repository data through a potentially public artifact,
// bypassing the enclave's finite disclosure contract.
//
// Redaction replaces payload content with metadata only — byte count and a
// stable, non-reversible digest — so tool names, directions, outcomes, sizes,
// timings, and DIFC decisions stay diagnosable while the content itself never
// reaches an exported artifact.

// EnvRawPayloadLogs is the privileged opt-in that restores raw payload logging
// for enclave-scoped sessions. It exists for local debugging only; enabling it
// in a workflow republishes enclave payloads into exported artifacts.
const EnvRawPayloadLogs = "MCP_GATEWAY_UNSAFE_RAW_ENCLAVE_PAYLOAD_LOGS"

// redactedPayloadDigestLen is the hex length of the payload digest included in
// redacted log entries. It is long enough to correlate repeated payloads across
// log lines and short enough to keep entries readable.
const redactedPayloadDigestLen = 16

// digestPrefix marks a token produced by KeyedDigest. The token is an HMAC, not
// a bare hash, so it is labelled distinctly from the plain SHA-256 tokens used
// for non-sensitive attribution elsewhere.
const digestPrefix = "hmac:"

var (
	payloadRedaction  atomic.Bool
	rawPayloadOnce    sync.Once
	rawPayloadAllowed atomic.Bool
	digestKeyOnce     sync.Once
	digestKey         []byte
)

// payloadDigestKey returns the per-process secret used to key redaction
// digests. A plain (even truncated) SHA-256 of a redacted value is recoverable
// by dictionary attack whenever the value comes from a small enumerable set —
// repository names, file paths, issue numbers, logins — and the digests are
// published in an artifact readable by anyone. Keying the digest with a random
// secret that never leaves the process preserves the only property logs need
// (equal values produce equal tokens within one run) while making the token
// useless to an artifact reader.
func payloadDigestKey() []byte {
	digestKeyOnce.Do(func() {
		key := make([]byte, sha256.Size)
		if _, err := rand.Read(key); err != nil {
			// Without a secret key, a digest would be attackable; emit no digest
			// at all rather than a guessable one.
			return
		}
		digestKey = key
	})
	return digestKey
}

// KeyedDigest returns a stable, non-reversible, per-process-keyed token for a
// value that must not be persisted in an exported log. Empty values render as
// "(none)". Tokens are comparable within a single process run and carry no
// information to a reader of the resulting artifact.
func KeyedDigest(value string) string {
	if value == "" {
		return "(none)"
	}
	key := payloadDigestKey()
	if key == nil {
		return digestPrefix + "unavailable"
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(value))
	return digestPrefix + hex.EncodeToString(mac.Sum(nil))[:redactedPayloadDigestLen]
}

// rawPayloadLogsAllowed reports whether the privileged raw-payload opt-in is
// set. It is resolved once per process because it gates a security default.
func rawPayloadLogsAllowed() bool {
	rawPayloadOnce.Do(func() {
		allowed, err := strconv.ParseBool(os.Getenv(EnvRawPayloadLogs))
		rawPayloadAllowed.Store(err == nil && allowed)
	})
	return rawPayloadAllowed.Load()
}

// SetRawPayloadLogsAllowed overrides the privileged raw-payload opt-in. It
// exists so tests can exercise both modes; production code resolves the value
// from the environment.
func SetRawPayloadLogsAllowed(allowed bool) {
	rawPayloadOnce.Do(func() {})
	rawPayloadAllowed.Store(allowed)
}

// EnablePayloadRedaction turns on process-wide redaction of MCP request and
// response payloads in every exported log sink. It is called during startup for
// the enclave and delegation profiles and is safe for concurrent use.
func EnablePayloadRedaction() {
	payloadRedaction.Store(true)
}

// SetPayloadRedaction sets the process-wide payload redaction mode explicitly.
// It exists so tests can restore the previous mode; production code should call
// EnablePayloadRedaction.
func SetPayloadRedaction(enabled bool) {
	payloadRedaction.Store(enabled)
}

// PayloadRedactionEnabled reports whether process-wide payload redaction is
// active. The privileged raw-payload opt-in disables it.
func PayloadRedactionEnabled() bool {
	return ShouldRedactPayload(false)
}

// ShouldRedactPayload reports whether a payload must be reduced to metadata
// before it is written to any log sink. enclaveSession carries the per-request
// enclave provenance marker, so a single enclave-scoped call is redacted even
// when the process default is off.
func ShouldRedactPayload(enclaveSession bool) bool {
	if rawPayloadLogsAllowed() {
		return false
	}
	return enclaveSession || payloadRedaction.Load()
}

// RawPayloadLogsAllowed reports whether the privileged raw-payload opt-in is
// set for this process. Callers use it to warn that enclave payloads will be
// written to exported artifacts.
func RawPayloadLogsAllowed() bool {
	return rawPayloadLogsAllowed()
}

// PayloadDigest returns a stable, non-reversible digest of payload suitable for
// correlating identical payloads across log lines within one process run.
func PayloadDigest(payload []byte) string {
	return KeyedDigest(string(payload))
}

// RedactedPayloadText returns the metadata-only rendering of payload used by the
// text and markdown log sinks.
func RedactedPayloadText(payload []byte) string {
	return fmt.Sprintf("[REDACTED enclave payload bytes=%d digest=%s]", len(payload), PayloadDigest(payload))
}

// redactedPayload is the metadata-only JSON object written in place of an
// enclave payload in the JSONL log.
type redactedPayload struct {
	Redacted bool   `json:"redacted"`
	Reason   string `json:"reason"`
	Bytes    int    `json:"bytes"`
	Digest   string `json:"digest"`
}

// RedactedPayloadJSON returns the metadata-only JSON document written in place
// of an enclave payload in the JSONL log.
func RedactedPayloadJSON(payload []byte) json.RawMessage {
	encoded, err := json.Marshal(redactedPayload{
		Redacted: true,
		Reason:   "enclave_payload_redacted",
		Bytes:    len(payload),
		Digest:   PayloadDigest(payload),
	})
	if err != nil {
		// json.Marshal of a fixed struct of scalars cannot fail; fall back to a
		// literal so a logging path can never emit unredacted content.
		return json.RawMessage(`{"redacted":true,"reason":"enclave_payload_redacted"}`)
	}
	return encoded
}

// RedactErrorForLog reduces an error to a coarse category plus a stable digest.
// Backend errors frequently embed response bodies, so the message itself cannot
// be persisted for enclave-scoped traffic.
func RedactErrorForLog(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "error " + KeyedDigest(err.Error())
	}
}

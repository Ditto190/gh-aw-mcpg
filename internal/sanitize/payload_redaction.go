package sanitize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/github/gh-aw-mcpg/internal/util"
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
// log lines and short enough to stay unusable as a content oracle.
const redactedPayloadDigestLen = 16

var (
	payloadRedaction  atomic.Bool
	rawPayloadOnce    sync.Once
	rawPayloadAllowed atomic.Bool
)

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
// correlating identical payloads across log lines.
func PayloadDigest(payload []byte) string {
	return util.HashForLog(string(payload), redactedPayloadDigestLen, "sha256:")
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
		return "error " + util.HashForLog(err.Error(), redactedPayloadDigestLen, "sha256:")
	}
}

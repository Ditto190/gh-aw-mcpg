package enclavegithub

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/github/gh-aw-mcpg/internal/logger"
)

var logCapability = logger.ForFile()

const (
	maxCapabilityTokenBytes = 16 * 1024
	maxInvocationIDBytes    = 256
)

// Claims is the invocation-bound capability payload minted by AWF.
type Claims struct {
	Version    int      `json:"v"`
	Audience   string   `json:"aud"`
	Run        string   `json:"run"`
	Invocation string   `json:"inv"`
	Repo       string   `json:"repo"`
	Profile    string   `json:"profile"`
	Operations []string `json:"ops"`
	NotBefore  int64    `json:"nbf"`
	Expires    int64    `json:"exp"`
}

// AgentID returns the stable DIFC registry key for this invocation.
func (c *Claims) AgentID() string {
	sum := sha256.Sum256([]byte(c.Run + "\x00" + c.Invocation))
	return "enclave:" + hex.EncodeToString(sum[:16])
}

// AllowsOperation reports whether the invocation capability permits operation.
func (c *Claims) AllowsOperation(operation string) bool {
	return slices.Contains(c.Operations, operation)
}

// Verifier verifies HMAC-signed invocation capabilities against one workflow policy.
type Verifier struct {
	key    []byte
	policy *Policy
}

// NewVerifier creates a capability verifier from a 32-byte lowercase-hex root key.
func NewVerifier(rootKeyHex string, policy *Policy) (*Verifier, error) {
	if len(rootKeyHex) != 64 || strings.ToLower(rootKeyHex) != rootKeyHex {
		return nil, fmt.Errorf("enclave capability key must be exactly 64 lowercase hex characters")
	}
	key, err := hex.DecodeString(rootKeyHex)
	if err != nil || len(key) != sha256.Size {
		return nil, fmt.Errorf("enclave capability key must be exactly 64 lowercase hex characters")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	logCapability.Printf("Created enclave capability verifier: audience=%s, run=%s, profile=%s", policy.Audience, policy.WorkflowRunID, policy.Profile)
	return &Verifier{key: key, policy: policy}, nil
}

// VerifyAuthorization verifies a Bearer capability at the current time.
func (v *Verifier) VerifyAuthorization(header string) (*Claims, error) {
	return v.verifyAuthorizationAt(header, time.Now())
}

func (v *Verifier) verifyAuthorizationAt(header string, now time.Time) (*Claims, error) {
	// Stock gh sends configured tokens as "token <value>"; both exact,
	// case-sensitive schemes carry the same capability.
	for _, prefix := range []string{"Bearer ", "token "} {
		if !strings.HasPrefix(header, prefix) {
			continue
		}
		token := header[len(prefix):]
		if len(token) == 0 || strings.ContainsAny(token, " \t\r\n") {
			logCapability.Print("Rejected enclave capability: malformed token after scheme prefix")
			return nil, fmt.Errorf("invalid enclave capability")
		}
		return v.verifyTokenAt(token, now)
	}
	logCapability.Print("Rejected enclave capability: missing Bearer/token authorization scheme")
	return nil, fmt.Errorf("invalid enclave capability")
}

func (v *Verifier) verifyTokenAt(token string, now time.Time) (*Claims, error) {
	if len(token) == 0 || len(token) > maxCapabilityTokenBytes {
		return nil, fmt.Errorf("invalid enclave capability")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != CapabilityPrefix {
		return nil, fmt.Errorf("invalid enclave capability")
	}

	signingInput := parts[0] + "." + parts[1]
	providedSignature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(providedSignature) != sha256.Size {
		return nil, fmt.Errorf("invalid enclave capability")
	}
	mac := hmac.New(sha256.New, v.key)
	_, _ = mac.Write([]byte(signingInput))
	expectedSignature := mac.Sum(nil)
	if subtle.ConstantTimeCompare(providedSignature, expectedSignature) != 1 {
		logCapability.Print("Rejected enclave capability: HMAC signature mismatch")
		return nil, fmt.Errorf("invalid enclave capability")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid enclave capability")
	}
	var claims Claims
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return nil, fmt.Errorf("invalid enclave capability")
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("invalid enclave capability")
	}
	if err := v.validateClaims(&claims, now); err != nil {
		logCapability.Printf("Rejected enclave capability: claims validation failed: %v", err)
		return nil, fmt.Errorf("invalid enclave capability")
	}
	logCapability.Printf("Verified enclave capability: agent=%s, invocation=%s, ops=%d", claims.AgentID(), claims.Invocation, len(claims.Operations))
	return &claims, nil
}

func (v *Verifier) validateClaims(claims *Claims, now time.Time) error {
	if claims.Version != 1 ||
		claims.Audience != v.policy.Audience ||
		claims.Run != v.policy.WorkflowRunID ||
		claims.Profile != v.policy.Profile {
		return fmt.Errorf("claims binding mismatch")
	}
	if claims.Invocation == "" || len(claims.Invocation) > maxInvocationIDBytes ||
		!invocationPattern.MatchString(claims.Invocation) {
		return fmt.Errorf("invalid invocation")
	}
	if !repositoryPattern.MatchString(claims.Repo) || !v.policy.HasRepository(claims.Repo) {
		return fmt.Errorf("invalid assigned repository")
	}
	if len(claims.Operations) == 0 {
		return fmt.Errorf("missing operations")
	}
	if !slices.IsSorted(claims.Operations) {
		return fmt.Errorf("operations must be lexicographically sorted")
	}
	seen := make(map[string]struct{}, len(claims.Operations))
	for _, operation := range claims.Operations {
		if !v.policy.AllowsOperation(operation) {
			return fmt.Errorf("operation outside policy")
		}
		if _, exists := seen[operation]; exists {
			return fmt.Errorf("duplicate operation")
		}
		seen[operation] = struct{}{}
	}
	if claims.NotBefore <= 0 || claims.Expires <= claims.NotBefore ||
		claims.Expires-claims.NotBefore > v.policy.MaxCapabilityTTLSeconds {
		return fmt.Errorf("invalid capability lifetime")
	}
	nowUnix := now.Unix()
	if nowUnix < claims.NotBefore || nowUnix >= claims.Expires {
		return fmt.Errorf("capability outside validity window")
	}
	return nil
}

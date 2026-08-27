package enclavegithub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validPolicyJSON() string {
	return `{
		"version":1,
		"profile":"issues-read-v1",
		"audience":"gh-aw-enclave-github",
		"workflow_run_id":"run-123",
		"repositories":[{"repo":"github/gh-aw","sensitivity":"confidential"}],
		"public_min_integrity":"approved",
		"allowed_operations":["issues.comments.list","issues.get","issues.list"],
		"max_capability_ttl_seconds":600
	}`
}

func TestParsePolicy(t *testing.T) {
	policy, err := ParsePolicy(validPolicyJSON())
	require.NoError(t, err)
	assert.Equal(t, []string{"github/gh-aw", "public"}, policy.GuardRepos())
	assert.True(t, policy.HasRepository("github/gh-aw"))
	assert.True(t, policy.AllowsOperation(OperationIssuesGet))
}

func TestParsePolicyPreservesCompilerProxyIdentity(t *testing.T) {
	const identity = "gh-aw-egh-123456-2-abcdef123456"
	raw := strings.Replace(validPolicyJSON(), `"run-123"`, `"`+identity+`"`, 1)
	policy, err := ParsePolicy(raw)

	require.NoError(t, err)
	assert.Equal(t, identity, policy.WorkflowRunID)
}

func TestParsePolicyRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"unknown field", `{"version":1,"extra":true}`},
		{"uppercase repo", `{"version":1,"profile":"issues-read-v1","audience":"gh-aw-enclave-github","workflow_run_id":"r","repositories":[{"repo":"GitHub/gh-aw","sensitivity":"confidential"}],"public_min_integrity":"approved","allowed_operations":["issues.get"],"max_capability_ttl_seconds":600}`},
		{"unknown operation", `{"version":1,"profile":"issues-read-v1","audience":"gh-aw-enclave-github","workflow_run_id":"r","repositories":[{"repo":"github/gh-aw","sensitivity":"confidential"}],"public_min_integrity":"approved","allowed_operations":["repos:get"],"max_capability_ttl_seconds":600}`},
		{"excessive ttl", `{"version":1,"profile":"issues-read-v1","audience":"gh-aw-enclave-github","workflow_run_id":"r","repositories":[{"repo":"github/gh-aw","sensitivity":"confidential"}],"public_min_integrity":"approved","allowed_operations":["issues.get"],"max_capability_ttl_seconds":601}`},
		{"unsorted operations", `{"version":1,"profile":"issues-read-v1","audience":"gh-aw-enclave-github","workflow_run_id":"r","repositories":[{"repo":"github/gh-aw","sensitivity":"confidential"}],"public_min_integrity":"approved","allowed_operations":["issues.list","issues.get"],"max_capability_ttl_seconds":600}`},
		{"noncanonical run identity", `{"version":1,"profile":"issues-read-v1","audience":"gh-aw-enclave-github","workflow_run_id":"Run_123","repositories":[{"repo":"github/gh-aw","sensitivity":"confidential"}],"public_min_integrity":"approved","allowed_operations":["issues.get"],"max_capability_ttl_seconds":600}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePolicy(tt.raw)
			require.Error(t, err)
		})
	}
}

func TestParsePolicyRejectsEmptyInput(t *testing.T) {
	_, err := ParsePolicy("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enclave policy is required")
}

func TestParsePolicyRejectsOversizedInput(t *testing.T) {
	huge := `{"padding":"` + strings.Repeat("a", maxPolicyJSONBytes) + `"}`
	_, err := ParsePolicy(huge)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestParsePolicyRejectsTrailingGarbage(t *testing.T) {
	// Valid policy JSON followed by a non-JSON trailing token must be rejected
	// by ensureJSONEOF's err != io.EOF, err != nil branch.
	raw := validPolicyJSON() + "not-json"
	_, err := ParsePolicy(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid enclave policy JSON")
}

func TestParsePolicyRejectsMultipleJSONValues(t *testing.T) {
	// Two back-to-back valid JSON values must be rejected by ensureJSONEOF's
	// err == nil branch (exactly one JSON value required).
	raw := validPolicyJSON() + `{}`
	_, err := ParsePolicy(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one JSON value")
}

func TestPolicyValidate_NilReceiver(t *testing.T) {
	var p *Policy
	err := p.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enclave policy is required")
}

func TestPolicyValidate_FieldErrors(t *testing.T) {
	validRepositories := []RepositoryPolicy{{Repo: "github/gh-aw", Sensitivity: "confidential"}}
	basePolicy := func() *Policy {
		return &Policy{
			Version:                 1,
			Profile:                 ProfileIssuesReadV1,
			Audience:                DefaultAudience,
			WorkflowRunID:           "run-123",
			Repositories:            append([]RepositoryPolicy(nil), validRepositories...),
			PublicMinIntegrity:      "approved",
			AllowedOperations:       []string{OperationIssuesGet},
			MaxCapabilityTTLSeconds: 600,
		}
	}

	tests := []struct {
		name      string
		mutate    func(*Policy)
		errSubstr string
	}{
		{"wrong profile", func(p *Policy) { p.Profile = "other-profile" }, "unsupported enclave profile"},
		{"wrong audience", func(p *Policy) { p.Audience = "other-audience" }, "unsupported enclave audience"},
		{"no repositories", func(p *Policy) { p.Repositories = nil }, "repositories must contain at least one entry"},
		{"duplicate repo", func(p *Policy) {
			p.Repositories = []RepositoryPolicy{
				{Repo: "github/gh-aw", Sensitivity: "confidential"},
				{Repo: "github/gh-aw", Sensitivity: "public"},
			}
		}, "must not contain duplicate repo"},
		{"invalid sensitivity", func(p *Policy) {
			p.Repositories = []RepositoryPolicy{{Repo: "github/gh-aw", Sensitivity: "bogus"}}
		}, "sensitivity is invalid"},
		{"invalid public_min_integrity", func(p *Policy) { p.PublicMinIntegrity = "bogus" }, "public_min_integrity must be one of"},
		{"no allowed operations", func(p *Policy) { p.AllowedOperations = nil }, "allowed_operations must contain at least one operation"},
		{"duplicate operation", func(p *Policy) {
			p.AllowedOperations = []string{OperationIssuesGet, OperationIssuesGet}
		}, "must not contain duplicate operation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := basePolicy()
			tt.mutate(policy)
			err := policy.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errSubstr)
		})
	}
}

func TestPolicyRepositorySensitivity(t *testing.T) {
	policy, err := ParsePolicy(validPolicyJSON())
	require.NoError(t, err)

	sensitivity, ok := policy.RepositorySensitivity("github/gh-aw")
	assert.True(t, ok)
	assert.Equal(t, "confidential", sensitivity)

	sensitivity, ok = policy.RepositorySensitivity("unknown/repo")
	assert.False(t, ok)
	assert.Empty(t, sensitivity)
}

func TestPolicyGuardPolicyJSON(t *testing.T) {
	policy, err := ParsePolicy(validPolicyJSON())
	require.NoError(t, err)

	encoded, err := policy.GuardPolicyJSON()
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(encoded), &decoded))
	allowOnly, ok := decoded["allow-only"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "approved", allowOnly["min-integrity"])
	assert.ElementsMatch(t, []interface{}{"github/gh-aw", "public"}, allowOnly["repos"])
}

func TestPolicyGuardPolicyJSONPropagatesValidationError(t *testing.T) {
	invalid := &Policy{Version: 2}
	_, err := invalid.GuardPolicyJSON()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version must be 1")
}

func TestVerifierValidatesInvocationBinding(t *testing.T) {
	policy, err := ParsePolicy(validPolicyJSON())
	require.NoError(t, err)
	keyHex := hex.EncodeToString(make([]byte, sha256.Size))
	verifier, err := NewVerifier(keyHex, policy)
	require.NoError(t, err)
	now := time.Unix(1_700_000_000, 0)
	claims := Claims{
		Version: 1, Audience: DefaultAudience, Run: "run-123", Invocation: "inv-1",
		Repo: "github/gh-aw", Profile: ProfileIssuesReadV1,
		Operations: []string{OperationIssuesGet}, NotBefore: now.Unix() - 1,
		Expires: now.Unix() + 60,
	}
	token := signClaims(t, make([]byte, sha256.Size), claims)

	got, err := verifier.verifyAuthorizationAt("Bearer "+token, now)
	require.NoError(t, err)
	assert.Equal(t, claims.Repo, got.Repo)
	assert.True(t, got.AllowsOperation(OperationIssuesGet))
	assert.NotContains(t, got.AgentID(), claims.Invocation)
}

func TestVerifierAcceptsExactAWFPositiveVector(t *testing.T) {
	const (
		keyHex     = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
		payload    = `{"v":1,"aud":"gh-aw-enclave-github","run":"run-123","inv":"inv-456","repo":"octo/private","profile":"issues-read-v1","ops":["issues.comments.list","issues.get","issues.list"],"nbf":1787594400,"exp":1787594520}`
		payloadB64 = "eyJ2IjoxLCJhdWQiOiJnaC1hdy1lbmNsYXZlLWdpdGh1YiIsInJ1biI6InJ1bi0xMjMiLCJpbnYiOiJpbnYtNDU2IiwicmVwbyI6Im9jdG8vcHJpdmF0ZSIsInByb2ZpbGUiOiJpc3N1ZXMtcmVhZC12MSIsIm9wcyI6WyJpc3N1ZXMuY29tbWVudHMubGlzdCIsImlzc3Vlcy5nZXQiLCJpc3N1ZXMubGlzdCJdLCJuYmYiOjE3ODc1OTQ0MDAsImV4cCI6MTc4NzU5NDUyMH0"
		mac        = "6fDSD-uxmi_fCZMOFmSuckdNBGx5qcWMI1HCgoXtA3o"
	)
	policy, err := ParsePolicy(`{
		"version":1,
		"profile":"issues-read-v1",
		"audience":"gh-aw-enclave-github",
		"workflow_run_id":"run-123",
		"repositories":[{"repo":"octo/private","sensitivity":"confidential"}],
		"public_min_integrity":"approved",
		"allowed_operations":["issues.comments.list","issues.get","issues.list"],
		"max_capability_ttl_seconds":120
	}`)
	require.NoError(t, err)
	verifier, err := NewVerifier(keyHex, policy)
	require.NoError(t, err)

	decodedPayload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	require.NoError(t, err)
	assert.Equal(t, payload, string(decodedPayload))

	token := CapabilityPrefix + "." + payloadB64 + "." + mac
	claims, err := verifier.verifyAuthorizationAt(
		"Bearer "+token,
		time.Unix(1_787_594_460, 0),
	)
	require.NoError(t, err)
	assert.Equal(t, "inv-456", claims.Invocation)
	assert.Equal(t, "run-123", claims.Run)
	assert.Equal(t, "octo/private", claims.Repo)
	assert.Equal(t, []string{
		OperationIssueCommentsList,
		OperationIssuesGet,
		OperationIssuesList,
	}, claims.Operations)
}

func TestVerifierRejectsAWFVectorWireMutations(t *testing.T) {
	const validToken = "awf-egh1.eyJ2IjoxLCJhdWQiOiJnaC1hdy1lbmNsYXZlLWdpdGh1YiIsInJ1biI6InJ1bi0xMjMiLCJpbnYiOiJpbnYtNDU2IiwicmVwbyI6Im9jdG8vcHJpdmF0ZSIsInByb2ZpbGUiOiJpc3N1ZXMtcmVhZC12MSIsIm9wcyI6WyJpc3N1ZXMuY29tbWVudHMubGlzdCIsImlzc3Vlcy5nZXQiLCJpc3N1ZXMubGlzdCJdLCJuYmYiOjE3ODc1OTQ0MDAsImV4cCI6MTc4NzU5NDUyMH0.6fDSD-uxmi_fCZMOFmSuckdNBGx5qcWMI1HCgoXtA3o"
	policy, err := ParsePolicy(`{
		"version":1,
		"profile":"issues-read-v1",
		"audience":"gh-aw-enclave-github",
		"workflow_run_id":"run-123",
		"repositories":[{"repo":"octo/private","sensitivity":"confidential"}],
		"public_min_integrity":"approved",
		"allowed_operations":["issues.comments.list","issues.get","issues.list"],
		"max_capability_ttl_seconds":120
	}`)
	require.NoError(t, err)
	verifier, err := NewVerifier(
		"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		policy,
	)
	require.NoError(t, err)

	mutations := []string{
		strings.Replace(validToken, CapabilityPrefix, "v1", 1),
		strings.Replace(validToken, "eyJ2I", "eyJ3I", 1),
		validToken[:len(validToken)-1] + "A",
	}
	for _, token := range mutations {
		_, err := verifier.verifyAuthorizationAt("Bearer "+token, time.Unix(1_787_594_460, 0))
		require.Error(t, err)
		assert.Equal(t, "invalid enclave capability", err.Error())
	}
}

func TestVerifierRejectsInvalidCapabilities(t *testing.T) {
	policy, err := ParsePolicy(validPolicyJSON())
	require.NoError(t, err)
	key := make([]byte, sha256.Size)
	verifier, err := NewVerifier(hex.EncodeToString(key), policy)
	require.NoError(t, err)
	now := time.Unix(1_700_000_000, 0)
	base := Claims{
		Version: 1, Audience: DefaultAudience, Run: "run-123", Invocation: "inv-1",
		Repo: "github/gh-aw", Profile: ProfileIssuesReadV1,
		Operations: []string{OperationIssuesGet}, NotBefore: now.Unix() - 1,
		Expires: now.Unix() + 60,
	}

	tests := []struct {
		name   string
		mutate func(*Claims)
	}{
		{"wrong version", func(c *Claims) { c.Version = 2 }},
		{"wrong run", func(c *Claims) { c.Run = "other" }},
		{"wrong run casing", func(c *Claims) { c.Run = "Run-123" }},
		{"wrong repo", func(c *Claims) { c.Repo = "github/private" }},
		{"wrong audience", func(c *Claims) { c.Audience = "other" }},
		{"wrong profile", func(c *Claims) { c.Profile = "other" }},
		{"wrong invocation", func(c *Claims) { c.Invocation = "" }},
		{"expired", func(c *Claims) { c.Expires = now.Unix() }},
		{"not yet valid", func(c *Claims) { c.NotBefore = now.Unix() + 1; c.Expires = now.Unix() + 60 }},
		{"operation escalation", func(c *Claims) { c.Operations = []string{"repos:get"} }},
		{"operation ordering", func(c *Claims) {
			c.Operations = []string{OperationIssuesList, OperationIssuesGet}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := base
			claims.Operations = append([]string(nil), base.Operations...)
			tt.mutate(&claims)
			_, err := verifier.verifyAuthorizationAt("Bearer "+signClaims(t, key, claims), now)
			require.Error(t, err)
			assert.Equal(t, "invalid enclave capability", err.Error())
		})
	}
}

func TestMatchRoute(t *testing.T) {
	tests := []struct {
		path      string
		query     url.Values
		operation string
		repo      string
	}{
		{"/repos/github/gh-aw/issues", url.Values{"state": {"open"}, "page": {"2"}}, OperationIssuesList, "github/gh-aw"},
		{"/repos/github/gh-aw/issues/12", nil, OperationIssuesGet, "github/gh-aw"},
		{"/repos/github/gh-aw/issues/12/comments", url.Values{"per_page": {"100"}}, OperationIssueCommentsList, "github/gh-aw"},
	}
	for _, tt := range tests {
		route, err := MatchRoute(tt.path, tt.query)
		require.NoError(t, err)
		assert.Equal(t, tt.operation, route.Operation)
		assert.Equal(t, tt.repo, route.FullRepo())
	}
}

func TestMatchRouteRejectsBroadSurface(t *testing.T) {
	tests := []struct {
		path  string
		query url.Values
	}{
		{"/graphql", nil},
		{"/search/issues", nil},
		{"/repos/github/gh-aw/pulls", nil},
		{"/repos/github/gh-aw/issues/0", nil},
		{"/repos/github/gh-aw/issues", url.Values{"unknown": {"value"}}},
		{"/repos/github/gh-aw/issues", url.Values{"page": {"1", "2"}}},
		{"/repos/GitHub/gh-aw/issues", nil},
		{"/repos/./repo/issues", nil},
	}
	for _, tt := range tests {
		_, err := MatchRoute(tt.path, tt.query)
		require.Error(t, err)
	}
}

func signClaims(t *testing.T, key []byte, claims Claims) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := CapabilityPrefix + "." + encoded
	mac := hmac.New(sha256.New, key)
	_, err = mac.Write([]byte(signingInput))
	require.NoError(t, err)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

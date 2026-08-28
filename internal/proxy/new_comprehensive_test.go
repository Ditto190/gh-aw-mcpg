package proxy

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/difc"
	"github.com/github/gh-aw-mcpg/internal/enclavegithub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullGuardWasmWithLabelAgentSuccess exports label_resource, label_response
// (both no-ops returning i32.const 0) and label_agent, which writes the JSON
// payload `{"difc_mode": "strict"}` into the caller-supplied output buffer
// and returns its length. This is the minimal WASM module that lets New's
// initGuardPolicy/LabelAgent success path run to completion (including the
// DIFCMode-override branch), which the existing fixture in new_test.go
// (fullGuardWasmForNewTest) cannot exercise because its label_agent always
// returns an empty response.
//
// Compiled by hand from:
//
//	(module
//	  (memory (export "memory") 1)
//	  (func (export "label_resource") (param i32 i32 i32 i32) (result i32) i32.const 0)
//	  (func (export "label_response") (param i32 i32 i32 i32) (result i32) i32.const 0)
//	  (func (export "label_agent") (param i32 i32 i32 i32) (result i32)
//	    ;; store each byte of {"difc_mode": "strict"} into memory at outPtr (param 2)
//	    ;; via i32.store8, then return i32.const 23 (the JSON length))
var fullGuardWasmWithLabelAgentSuccess = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0x09, 0x01, 0x60, 0x04, 0x7f, 0x7f, 0x7f,
	0x7f, 0x01, 0x7f, 0x03, 0x04, 0x03, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01, 0x07, 0x3a,
	0x04, 0x0e, 0x6c, 0x61, 0x62, 0x65, 0x6c, 0x5f, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x00, 0x00, 0x0e, 0x6c, 0x61, 0x62, 0x65, 0x6c, 0x5f, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x00, 0x01, 0x0b, 0x6c, 0x61, 0x62, 0x65, 0x6c, 0x5f, 0x61, 0x67, 0x65, 0x6e, 0x74, 0x00,
	0x02, 0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00, 0x0a, 0xc3, 0x01, 0x03, 0x04, 0x00,
	0x41, 0x00, 0x0b, 0x04, 0x00, 0x41, 0x00, 0x0b, 0xb6, 0x01, 0x00, 0x20, 0x02, 0x41, 0xfb, 0x00,
	0x3a, 0x00, 0x00, 0x20, 0x02, 0x41, 0x22, 0x3a, 0x00, 0x01, 0x20, 0x02, 0x41, 0xe4, 0x00, 0x3a,
	0x00, 0x02, 0x20, 0x02, 0x41, 0xe9, 0x00, 0x3a, 0x00, 0x03, 0x20, 0x02, 0x41, 0xe6, 0x00, 0x3a,
	0x00, 0x04, 0x20, 0x02, 0x41, 0xe3, 0x00, 0x3a, 0x00, 0x05, 0x20, 0x02, 0x41, 0xdf, 0x00, 0x3a,
	0x00, 0x06, 0x20, 0x02, 0x41, 0xed, 0x00, 0x3a, 0x00, 0x07, 0x20, 0x02, 0x41, 0xef, 0x00, 0x3a,
	0x00, 0x08, 0x20, 0x02, 0x41, 0xe4, 0x00, 0x3a, 0x00, 0x09, 0x20, 0x02, 0x41, 0xe5, 0x00, 0x3a,
	0x00, 0x0a, 0x20, 0x02, 0x41, 0x22, 0x3a, 0x00, 0x0b, 0x20, 0x02, 0x41, 0x3a, 0x3a, 0x00, 0x0c,
	0x20, 0x02, 0x41, 0x20, 0x3a, 0x00, 0x0d, 0x20, 0x02, 0x41, 0x22, 0x3a, 0x00, 0x0e, 0x20, 0x02,
	0x41, 0xf3, 0x00, 0x3a, 0x00, 0x0f, 0x20, 0x02, 0x41, 0xf4, 0x00, 0x3a, 0x00, 0x10, 0x20, 0x02,
	0x41, 0xf2, 0x00, 0x3a, 0x00, 0x11, 0x20, 0x02, 0x41, 0xe9, 0x00, 0x3a, 0x00, 0x12, 0x20, 0x02,
	0x41, 0xe3, 0x00, 0x3a, 0x00, 0x13, 0x20, 0x02, 0x41, 0xf4, 0x00, 0x3a, 0x00, 0x14, 0x20, 0x02,
	0x41, 0x22, 0x3a, 0x00, 0x15, 0x20, 0x02, 0x41, 0xfd, 0x00, 0x3a, 0x00, 0x16, 0x41, 0x17, 0x0b,
}

// writeSuccessGuardWasm writes fullGuardWasmWithLabelAgentSuccess to a temp
// file and returns its path.
func writeSuccessGuardWasm(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	wasmPath := filepath.Join(tmpDir, "guard.wasm")
	require.NoError(t, os.WriteFile(wasmPath, fullGuardWasmWithLabelAgentSuccess, 0o600))
	return wasmPath
}

// validEnclavePolicyJSON is a canonical enclave policy accepted by
// enclavegithub.ParsePolicy/Policy.Validate.
const validEnclavePolicyJSON = `{
	"version": 1,
	"profile": "issues-read-v1",
	"audience": "gh-aw-enclave-github",
	"workflow_run_id": "run-123",
	"repositories": [{"repo": "assigned/private", "sensitivity": "confidential"}],
	"public_min_integrity": "approved",
	"allowed_operations": ["issues.comments.list", "issues.get", "issues.list"],
	"max_capability_ttl_seconds": 600
}`

// newValidEnclavePolicyAndVerifier builds a real *enclavegithub.Policy and
// *enclavegithub.Verifier pair for use in enclave-mode New() tests.
func newValidEnclavePolicyAndVerifier(t *testing.T) (*enclavegithub.Policy, *enclavegithub.Verifier) {
	t.Helper()
	policy, err := enclavegithub.ParsePolicy(validEnclavePolicyJSON)
	require.NoError(t, err)
	key := make([]byte, 32)
	for i := range key {
		key[i] = 0x42
	}
	verifier, err := enclavegithub.NewVerifier(hex.EncodeToString(key), policy)
	require.NoError(t, err)
	return policy, verifier
}

// TestNew_PolicySuccess_OverridesDIFCMode verifies the full success path of
// initGuardPolicy from within New: when the guard's label_agent response
// includes a "difc_mode" field ("strict") that differs from the default
// filter mode, New adopts the overridden mode and marks guardInitialized.
// This covers the previously-uncovered success branches at the end of
// initGuardPolicy (result.DIFCMode != "" override) as well as New's
// "Initializing guard policy from config" log line and success return.
func TestNew_PolicySuccess_OverridesDIFCMode(t *testing.T) {
	wasmPath := writeSuccessGuardWasm(t)

	s, err := New(context.Background(), Config{
		WasmPath: wasmPath,
		Policy:   `{"allow-only":{"repos":"public","min-integrity":"none"}}`,
	})

	require.NoError(t, err)
	require.NotNil(t, s)
	wasmGuard, ok := s.guard.(interface {
		Close(context.Context) error
	})
	require.True(t, ok)
	t.Cleanup(func() {
		require.NoError(t, wasmGuard.Close(context.Background()))
	})

	assert.True(t, s.guardInitialized, "guardInitialized should be true after a successful LabelAgent call")
	assert.Equal(t, difc.EnforcementStrict, s.Mode, "New should adopt the DIFC mode overridden by the guard's label_agent response")
}

// TestNew_PolicySuccess_WithTrustedBotsAndUsers verifies that a successful
// policy initialization also works when TrustedBots and TrustedUsers are
// supplied, exercising the parameter-forwarding statement together with a
// real (non-error) LabelAgent completion.
func TestNew_PolicySuccess_WithTrustedBotsAndUsers(t *testing.T) {
	wasmPath := writeSuccessGuardWasm(t)

	s, err := New(context.Background(), Config{
		WasmPath:     wasmPath,
		Policy:       `{"allow-only":{"repos":"public","min-integrity":"none"}}`,
		TrustedBots:  []string{"my-bot[bot]"},
		TrustedUsers: []string{"octocat"},
	})

	require.NoError(t, err)
	require.NotNil(t, s)
	wasmGuard, ok := s.guard.(interface {
		Close(context.Context) error
	})
	require.True(t, ok)
	t.Cleanup(func() {
		require.NoError(t, wasmGuard.Close(context.Background()))
	})
	assert.True(t, s.guardInitialized)
}

// TestNew_EnclaveConfig_NilPolicyOrVerifier verifies that New rejects an
// Enclave config missing either the Policy or the Verifier, covering the
// "enclave policy and verifier are required" branch, for both possible nil
// fields independently.
func TestNew_EnclaveConfig_NilPolicyOrVerifier(t *testing.T) {
	wasmPath := writeSuccessGuardWasm(t)
	_, verifier := newValidEnclavePolicyAndVerifier(t)

	t.Run("nil policy", func(t *testing.T) {
		s, err := New(context.Background(), Config{
			WasmPath:    wasmPath,
			GitHubToken: "tok",
			Enclave: &EnclaveConfig{
				Policy:   nil,
				Verifier: verifier,
			},
		})
		require.Error(t, err)
		assert.Nil(t, s)
		assert.ErrorContains(t, err, "enclave policy and verifier are required")
	})

	t.Run("nil verifier", func(t *testing.T) {
		policy, _ := newValidEnclavePolicyAndVerifier(t)
		s, err := New(context.Background(), Config{
			WasmPath:    wasmPath,
			GitHubToken: "tok",
			Enclave: &EnclaveConfig{
				Policy:   policy,
				Verifier: nil,
			},
		})
		require.Error(t, err)
		assert.Nil(t, s)
		assert.ErrorContains(t, err, "enclave policy and verifier are required")
	})
}

// TestNew_EnclaveConfig_InvalidPolicy verifies that New propagates a
// validation error when the enclave policy itself is invalid, covering the
// "invalid enclave policy" error-wrapping branch.
func TestNew_EnclaveConfig_InvalidPolicy(t *testing.T) {
	wasmPath := writeSuccessGuardWasm(t)
	_, verifier := newValidEnclavePolicyAndVerifier(t)

	invalidPolicy := &enclavegithub.Policy{} // Version 0, missing required fields.

	s, err := New(context.Background(), Config{
		WasmPath:    wasmPath,
		GitHubToken: "tok",
		Enclave: &EnclaveConfig{
			Policy:   invalidPolicy,
			Verifier: verifier,
		},
	})
	require.Error(t, err)
	assert.Nil(t, s)
	assert.ErrorContains(t, err, "invalid enclave policy")
}

// TestNew_EnclaveConfig_MissingGitHubToken verifies that New requires a
// GitHubToken when enclave mode is configured, covering the "GitHub token is
// required for enclave proxy mode" branch.
func TestNew_EnclaveConfig_MissingGitHubToken(t *testing.T) {
	wasmPath := writeSuccessGuardWasm(t)
	policy, verifier := newValidEnclavePolicyAndVerifier(t)

	s, err := New(context.Background(), Config{
		WasmPath: wasmPath,
		Enclave: &EnclaveConfig{
			Policy:   policy,
			Verifier: verifier,
		},
	})
	require.Error(t, err)
	assert.Nil(t, s)
	assert.ErrorContains(t, err, "GitHub token is required for enclave proxy mode")
}

// TestNew_EnclaveConfig_RequiresGuardInitialized verifies that New rejects
// enclave mode when no guard policy was configured (so guardInitialized
// remains false), covering the "guard policy is required for enclave proxy
// mode" branch.
func TestNew_EnclaveConfig_RequiresGuardInitialized(t *testing.T) {
	wasmPath := writeSuccessGuardWasm(t)
	policy, verifier := newValidEnclavePolicyAndVerifier(t)

	s, err := New(context.Background(), Config{
		WasmPath:    wasmPath,
		GitHubToken: "tok",
		// No Policy set, so guardInitialized stays false.
		Enclave: &EnclaveConfig{
			Policy:   policy,
			Verifier: verifier,
		},
	})
	require.Error(t, err)
	assert.Nil(t, s)
	assert.ErrorContains(t, err, "guard policy is required for enclave proxy mode")
}

// TestNew_EnclaveConfig_Success verifies the full success path when enclave
// mode is enabled together with a valid guard policy: New should mark
// guardInitialized, retrieve the proxy agent's initial labels, and force the
// enforcement mode to propagate. This covers the previously-uncovered
// success tail of New (AgentRegistry.Get, SetDefaultLabels, Mode override,
// Evaluator.SetMode, and the CheckRedirect assignment for enclave mode).
func TestNew_EnclaveConfig_Success(t *testing.T) {
	wasmPath := writeSuccessGuardWasm(t)
	policy, verifier := newValidEnclavePolicyAndVerifier(t)

	s, err := New(context.Background(), Config{
		WasmPath:    wasmPath,
		Policy:      `{"allow-only":{"repos":"public","min-integrity":"none"}}`,
		GitHubToken: "tok",
		Enclave: &EnclaveConfig{
			Policy:   policy,
			Verifier: verifier,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, s)
	wasmGuard, ok := s.guard.(interface {
		Close(context.Context) error
	})
	require.True(t, ok)
	t.Cleanup(func() {
		require.NoError(t, wasmGuard.Close(context.Background()))
	})

	assert.True(t, s.guardInitialized)
	require.NotNil(t, s.enclave, "enclave state should be initialized")
	assert.Equal(t, difc.EnforcementPropagate, s.Mode, "enclave mode should force propagate enforcement")
	assert.NotNil(t, s.httpClient.CheckRedirect, "enclave mode should install a CheckRedirect that stops at the first response")

	labels, found := s.AgentRegistry.Get(proxyAgentID)
	require.True(t, found, "proxy agent labels should exist after successful enclave initialization")
	assert.NotNil(t, labels)
}

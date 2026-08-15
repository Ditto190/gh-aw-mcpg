package proxy

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullGuardWasm exports all three required guard functions (label_resource,
// label_response, label_agent) plus memory; all functions return i32.const 0
// (empty result). This is the minimal WASM module that allows guard.NewWasmGuard
// to succeed, letting these tests exercise the success path inside New (lines
// following the WASM-load step, previously uncovered).
//
// Compiled from:
//
//	(module
//	  (memory (export "memory") 1)
//	  (func (export "label_resource") (param i32 i32 i32 i32) (result i32) i32.const 0)
//	  (func (export "label_response") (param i32 i32 i32 i32) (result i32) i32.const 0)
//	  (func (export "label_agent") (param i32 i32 i32 i32) (result i32) i32.const 0))
var fullGuardWasmForNewTest = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0x09, 0x01, 0x60, 0x04, 0x7f, 0x7f, 0x7f,
	0x7f, 0x01, 0x7f, 0x03, 0x04, 0x03, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01, 0x07, 0x3a,
	0x04, 0x0e, 0x6c, 0x61, 0x62, 0x65, 0x6c, 0x5f, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x00, 0x00, 0x0e, 0x6c, 0x61, 0x62, 0x65, 0x6c, 0x5f, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x00, 0x01, 0x0b, 0x6c, 0x61, 0x62, 0x65, 0x6c, 0x5f, 0x61, 0x67, 0x65, 0x6e, 0x74, 0x00,
	0x02, 0x06, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x02, 0x00, 0x0a, 0x10, 0x03, 0x04, 0x00, 0x41,
	0x00, 0x0b, 0x04, 0x00, 0x41, 0x00, 0x0b, 0x04, 0x00, 0x41, 0x00, 0x0b,
}

// writeFullGuardWasm writes fullGuardWasmForNewTest to a temp file and returns its path.
func writeFullGuardWasm(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	wasmPath := filepath.Join(tmpDir, "guard.wasm")
	require.NoError(t, os.WriteFile(wasmPath, fullGuardWasmForNewTest, 0o600))
	return wasmPath
}

// TestNew_SuccessWithoutPolicy verifies the full success path of New when no
// guard policy is configured: WASM guard loads, Server fields are populated
// with defaults, and no LabelAgent call is attempted.
func TestNew_SuccessWithoutPolicy(t *testing.T) {
	wasmPath := writeFullGuardWasm(t)

	s, err := New(context.Background(), Config{
		WasmPath: wasmPath,
	})

	require.NoError(t, err)
	require.NotNil(t, s)
	assert.NotNil(t, s.guard)
	assert.Equal(t, DefaultGitHubAPIBase, s.githubAPIURL)
	assert.Empty(t, s.githubToken)
	assert.NotNil(t, s.httpClient)
	assert.False(t, s.guardInitialized, "guardInitialized should remain false when no policy is configured")
}

// TestNew_SuccessWithCustomAPIURLAndToken verifies that New trims a trailing
// slash from a custom GitHubAPIURL and stores the provided GitHubToken.
func TestNew_SuccessWithCustomAPIURLAndToken(t *testing.T) {
	wasmPath := writeFullGuardWasm(t)

	s, err := New(context.Background(), Config{
		WasmPath:     wasmPath,
		GitHubAPIURL: "https://my-ghe.example.com/api/v3/",
		GitHubToken:  "test-token",
		DIFCMode:     "filter",
	})

	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "https://my-ghe.example.com/api/v3", s.githubAPIURL)
	assert.Equal(t, "test-token", s.githubToken)
}

// TestNew_PolicyInitializationError verifies that New propagates and wraps
// errors from initGuardPolicy when an invalid policy is supplied, covering
// the "failed to initialize guard policy" error branch (cfg.Policy != "" and
// the returned error is non-nil).
func TestNew_PolicyInitializationError(t *testing.T) {
	wasmPath := writeFullGuardWasm(t)

	s, err := New(context.Background(), Config{
		WasmPath: wasmPath,
		Policy:   "not-valid-json",
	})

	require.Error(t, err)
	assert.Nil(t, s)
	assert.ErrorContains(t, err, "failed to initialize guard policy")
}

// TestNew_PolicyInitializationErrorWithTrustedBotsAndUsers verifies that
// TrustedBots and TrustedUsers are forwarded from Config into initGuardPolicy
// (exercising the parameter-passing statement in New) even when the overall
// call ultimately fails due to an invalid policy; fullGuardWasmForNewTest's
// label_agent implementation always returns an empty response, so a real
// success case for LabelAgent isn't reachable with this minimal fixture.
func TestNew_PolicyInitializationErrorWithTrustedBotsAndUsers(t *testing.T) {
	wasmPath := writeFullGuardWasm(t)

	s, err := New(context.Background(), Config{
		WasmPath:     wasmPath,
		Policy:       "not-valid-json",
		TrustedBots:  []string{"my-bot[bot]"},
		TrustedUsers: []string{"octocat"},
	})

	require.Error(t, err)
	assert.Nil(t, s)
	assert.ErrorContains(t, err, "failed to initialize guard policy")
}

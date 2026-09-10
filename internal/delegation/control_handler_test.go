package delegation

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newControlTestDeps builds a ControlDeps backed by a fresh Store and a
// state path in a temp directory, plus the capability secret needed to
// authenticate requests.
func newControlTestDeps(t *testing.T) (ControlDeps, string) {
	t.Helper()
	store, _ := newTestStore(t)
	capability, err := NewControlCapability("0123456789012345678901234567890123")
	require.NoError(t, err)
	statePath := filepath.Join(t.TempDir(), "state.json")
	return ControlDeps{
		Store:      store,
		Capability: capability,
		StatePath:  statePath,
	}, "0123456789012345678901234567890123"
}

func doControlRequest(t *testing.T, deps ControlDeps, secret, path, method string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if secret != "" {
		req.Header.Set("Authorization", secret)
	}
	w := httptest.NewRecorder()
	HandleControl(w, req, deps, nil)
	return w
}

func TestHandleControl_RejectsNonPOSTMethod(t *testing.T) {
	deps, secret := newControlTestDeps(t)
	w := doControlRequest(t, deps, secret, ControlPathPrefix+"status", http.MethodGet, struct{}{})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandleControl_RejectsMissingCapability(t *testing.T) {
	deps, _ := newControlTestDeps(t)
	w := doControlRequest(t, deps, "", ControlPathPrefix+"status", http.MethodPost, struct{}{})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandleControl_RejectsWrongCapability(t *testing.T) {
	deps, _ := newControlTestDeps(t)
	w := doControlRequest(t, deps, "wrong-secret-that-is-long-enough-000", ControlPathPrefix+"status", http.MethodPost, struct{}{})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandleControl_UnknownPathReturns404(t *testing.T) {
	deps, secret := newControlTestDeps(t)
	w := doControlRequest(t, deps, secret, ControlPathPrefix+"does-not-exist", http.MethodPost, struct{}{})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleControl_CustomLoggerIsInvoked(t *testing.T) {
	deps, secret := newControlTestDeps(t)
	var logged []string
	logf := func(format string, args ...any) {
		logged = append(logged, format)
	}
	req := httptest.NewRequest(http.MethodPost, ControlPathPrefix+"status", bytes.NewReader(mustJSON(t, map[string]string{"run_id": "run-123", "enclave_entry_id": "entry-1"})))
	req.Header.Set("Authorization", secret)
	w := httptest.NewRecorder()
	HandleControl(w, req, deps, logf)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, logged)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func TestHandleControl_CreateOrConfirm(t *testing.T) {
	t.Run("success creates a new identity and persists state", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		wire := CreateOrConfirmRequestWire{
			RunID:               "run-123",
			EnclaveBackend:      "awf-enclave",
			EnclaveEntryID:      "entry-1",
			InvocationID:        "inv-1",
			Repository:          "github/gh-aw",
			ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:          "sha256:abc",
			RequestedTTLSeconds: 60,
			IdempotencyKey:      "idem-1",
		}
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire)
		require.Equal(t, http.StatusOK, w.Code)

		var result IdentityResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		assert.NotEmpty(t, result.Handle)
		assert.NotEmpty(t, result.ExecutorBearer)
		assert.Equal(t, "github/gh-aw", result.Repository)

		// State must have been persisted to disk.
		_, err := os.Stat(deps.StatePath)
		require.NoError(t, err)
	})

	t.Run("invalid wire fields (bad requested_ttl) return 400", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		wire := CreateOrConfirmRequestWire{
			RunID:               "run-123",
			EnclaveBackend:      "awf-enclave",
			EnclaveEntryID:      "entry-1",
			InvocationID:        "inv-1",
			Repository:          "github/gh-aw",
			ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:          "sha256:abc",
			RequestedTTLSeconds: 0, // invalid: must be positive
			IdempotencyKey:      "idem-1",
		}
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("malformed JSON body returns 400", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		req := httptest.NewRequest(http.MethodPost, ControlPathPrefix+"create-or-confirm", bytes.NewReader([]byte("{not json")))
		req.Header.Set("Authorization", secret)
		w := httptest.NewRecorder()
		HandleControl(w, req, deps, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unknown JSON field returns 400", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		req := httptest.NewRequest(http.MethodPost, ControlPathPrefix+"create-or-confirm", bytes.NewReader([]byte(`{"run_id":"x","unknown_field":true}`)))
		req.Header.Set("Authorization", secret)
		w := httptest.NewRecorder()
		HandleControl(w, req, deps, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("trailing data after JSON object returns 400", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		req := httptest.NewRequest(http.MethodPost, ControlPathPrefix+"create-or-confirm", bytes.NewReader([]byte(`{"run_id":"x"}trailing`)))
		req.Header.Set("Authorization", secret)
		w := httptest.NewRecorder()
		HandleControl(w, req, deps, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("store rejection (mismatched idempotent replay) returns 403 and still persists state", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		wire := CreateOrConfirmRequestWire{
			RunID:               "run-123",
			EnclaveBackend:      "awf-enclave",
			EnclaveEntryID:      "entry-1",
			InvocationID:        "inv-1",
			Repository:          "github/gh-aw",
			ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:          "sha256:abc",
			RequestedTTLSeconds: 60,
			IdempotencyKey:      "idem-1",
		}
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire)
		require.Equal(t, http.StatusOK, w.Code)

		mismatched := wire
		mismatched.Repository = "github/gh-aw-firewall"
		w2 := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, mismatched)
		assert.Equal(t, http.StatusForbidden, w2.Code)

		_, err := os.Stat(deps.StatePath)
		require.NoError(t, err)
	})

	t.Run("persist failure after successful CreateOrConfirm returns 500", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		// Point StatePath at a directory (not a file) so SaveState fails to write.
		badDir := filepath.Join(t.TempDir(), "not-a-file")
		require.NoError(t, os.MkdirAll(badDir, 0o755))
		deps.StatePath = badDir

		wire := CreateOrConfirmRequestWire{
			RunID:               "run-123",
			EnclaveBackend:      "awf-enclave",
			EnclaveEntryID:      "entry-1",
			InvocationID:        "inv-1",
			Repository:          "github/gh-aw",
			ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:          "sha256:abc",
			RequestedTTLSeconds: 60,
			IdempotencyKey:      "idem-1",
		}
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("persist failure after store rejection still returns the persist failure status", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		wire := CreateOrConfirmRequestWire{
			RunID:               "run-123",
			EnclaveBackend:      "awf-enclave",
			EnclaveEntryID:      "entry-1",
			InvocationID:        "inv-1",
			Repository:          "github/gh-aw",
			ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:          "sha256:abc",
			RequestedTTLSeconds: 60,
			IdempotencyKey:      "idem-1",
		}
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire)
		require.Equal(t, http.StatusOK, w.Code)

		// Now break the state path so the mismatch-path persistState call fails.
		badDir := filepath.Join(t.TempDir(), "not-a-file")
		require.NoError(t, os.MkdirAll(badDir, 0o755))
		deps.StatePath = badDir

		mismatched := wire
		mismatched.Repository = "github/gh-aw-firewall"
		w2 := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, mismatched)
		assert.Equal(t, http.StatusInternalServerError, w2.Code)
	})
}

func TestHandleControl_Revoke(t *testing.T) {
	t.Run("success revokes an existing handle and persists state", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		wire := CreateOrConfirmRequestWire{
			RunID:               "run-123",
			EnclaveBackend:      "awf-enclave",
			EnclaveEntryID:      "entry-1",
			InvocationID:        "inv-1",
			Repository:          "github/gh-aw",
			ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:          "sha256:abc",
			RequestedTTLSeconds: 60,
			IdempotencyKey:      "idem-1",
		}
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire)
		require.Equal(t, http.StatusOK, w.Code)
		var result IdentityResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

		w2 := doControlRequest(t, deps, secret, ControlPathPrefix+"revoke", http.MethodPost, map[string]string{"handle": result.Handle})
		require.Equal(t, http.StatusOK, w2.Code)
		var revokeResp map[string]bool
		require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &revokeResp))
		assert.True(t, revokeResp["revoked"])
	})

	t.Run("revoking an unknown handle is idempotent and returns 200", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"revoke", http.MethodPost, map[string]string{"handle": "does-not-exist"})
		require.Equal(t, http.StatusOK, w.Code)
		var revokeResp map[string]bool
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &revokeResp))
		assert.True(t, revokeResp["revoked"])
	})

	t.Run("persist failure after successful revoke returns 500", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		wire := CreateOrConfirmRequestWire{
			RunID:               "run-123",
			EnclaveBackend:      "awf-enclave",
			EnclaveEntryID:      "entry-1",
			InvocationID:        "inv-1",
			Repository:          "github/gh-aw",
			ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:          "sha256:abc",
			RequestedTTLSeconds: 60,
			IdempotencyKey:      "idem-1",
		}
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire)
		require.Equal(t, http.StatusOK, w.Code)
		var result IdentityResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

		badDir := filepath.Join(t.TempDir(), "not-a-file")
		require.NoError(t, os.MkdirAll(badDir, 0o755))
		deps.StatePath = badDir

		w2 := doControlRequest(t, deps, secret, ControlPathPrefix+"revoke", http.MethodPost, map[string]string{"handle": result.Handle})
		assert.Equal(t, http.StatusInternalServerError, w2.Code)
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		req := httptest.NewRequest(http.MethodPost, ControlPathPrefix+"revoke", bytes.NewReader([]byte("not json")))
		req.Header.Set("Authorization", secret)
		w := httptest.NewRecorder()
		HandleControl(w, req, deps, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleControl_RevokeByLabels(t *testing.T) {
	t.Run("revokes zero or more matching identities and persists state", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		wire := CreateOrConfirmRequestWire{
			RunID:               "run-123",
			EnclaveBackend:      "awf-enclave",
			EnclaveEntryID:      "entry-1",
			InvocationID:        "inv-1",
			Repository:          "github/gh-aw",
			ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:          "sha256:abc",
			RequestedTTLSeconds: 60,
			IdempotencyKey:      "idem-1",
		}
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire)
		require.Equal(t, http.StatusOK, w.Code)

		w2 := doControlRequest(t, deps, secret, ControlPathPrefix+"revoke-by-labels", http.MethodPost, map[string]string{
			"run_id":           "run-123",
			"enclave_entry_id": "entry-1",
		})
		require.Equal(t, http.StatusOK, w2.Code)
		var resp map[string]int
		require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp["revoked"])
	})

	t.Run("no matches revokes zero and still succeeds", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"revoke-by-labels", http.MethodPost, map[string]string{
			"run_id":           "no-such-run",
			"enclave_entry_id": "no-such-entry",
		})
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]int
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp["revoked"])
	})

	t.Run("persist failure returns 500", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		badDir := filepath.Join(t.TempDir(), "not-a-file")
		require.NoError(t, os.MkdirAll(badDir, 0o755))
		deps.StatePath = badDir
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"revoke-by-labels", http.MethodPost, map[string]string{
			"run_id":           "run-123",
			"enclave_entry_id": "entry-1",
		})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		req := httptest.NewRequest(http.MethodPost, ControlPathPrefix+"revoke-by-labels", bytes.NewReader([]byte("not json")))
		req.Header.Set("Authorization", secret)
		w := httptest.NewRecorder()
		HandleControl(w, req, deps, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleControl_Status(t *testing.T) {
	t.Run("success returns store status and labelled handles", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		wire := CreateOrConfirmRequestWire{
			RunID:               "run-123",
			EnclaveBackend:      "awf-enclave",
			EnclaveEntryID:      "entry-1",
			InvocationID:        "inv-1",
			Repository:          "github/gh-aw",
			ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
			SchemaHash:          "sha256:abc",
			RequestedTTLSeconds: 60,
			IdempotencyKey:      "idem-1",
		}
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"create-or-confirm", http.MethodPost, wire)
		require.Equal(t, http.StatusOK, w.Code)

		w2 := doControlRequest(t, deps, secret, ControlPathPrefix+"status", http.MethodPost, map[string]string{
			"run_id":           "run-123",
			"enclave_entry_id": "entry-1",
		})
		require.Equal(t, http.StatusOK, w2.Code)
		var resp map[string]any
		require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
		assert.InDelta(t, float64(1), resp["generation"], 0)
		assert.InDelta(t, float64(1), resp["live_identity_count"], 0)
		assert.Equal(t, false, resp["recovery_incomplete"])
		handles, ok := resp["labelled_handles"].([]any)
		require.True(t, ok)
		assert.Len(t, handles, 1)
	})

	t.Run("missing run_id or enclave_entry_id returns 400", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)

		w := doControlRequest(t, deps, secret, ControlPathPrefix+"status", http.MethodPost, map[string]string{
			"run_id": "run-123",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)

		w2 := doControlRequest(t, deps, secret, ControlPathPrefix+"status", http.MethodPost, map[string]string{
			"enclave_entry_id": "entry-1",
		})
		assert.Equal(t, http.StatusBadRequest, w2.Code)

		w3 := doControlRequest(t, deps, secret, ControlPathPrefix+"status", http.MethodPost, map[string]string{})
		assert.Equal(t, http.StatusBadRequest, w3.Code)
	})

	t.Run("malformed body returns 400", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		req := httptest.NewRequest(http.MethodPost, ControlPathPrefix+"status", bytes.NewReader([]byte("not json")))
		req.Header.Set("Authorization", secret)
		w := httptest.NewRecorder()
		HandleControl(w, req, deps, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandleControl_Reconcile(t *testing.T) {
	t.Run("success clears recovery-incomplete and persists state", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		deps.Store.recoveryIncomplete = true
		require.True(t, deps.Store.IsRecoveryIncomplete())
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"reconcile", http.MethodPost, struct{}{})
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]bool
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp["reconciled"])
		assert.False(t, deps.Store.IsRecoveryIncomplete())

		_, err := os.Stat(deps.StatePath)
		require.NoError(t, err)
	})

	t.Run("persist failure returns 500", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		badDir := filepath.Join(t.TempDir(), "not-a-file")
		require.NoError(t, os.MkdirAll(badDir, 0o755))
		deps.StatePath = badDir
		w := doControlRequest(t, deps, secret, ControlPathPrefix+"reconcile", http.MethodPost, struct{}{})
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("unknown fields in the (empty) request body return 400", func(t *testing.T) {
		deps, secret := newControlTestDeps(t)
		req := httptest.NewRequest(http.MethodPost, ControlPathPrefix+"reconcile", bytes.NewReader([]byte(`{"unexpected":true}`)))
		req.Header.Set("Authorization", secret)
		w := httptest.NewRecorder()
		HandleControl(w, req, deps, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

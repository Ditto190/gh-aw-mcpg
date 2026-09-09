package delegation

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvelopeWire_MaxIdentityTTLIsSeconds(t *testing.T) {
	var wire EnvelopeWire
	require.NoError(t, json.Unmarshal([]byte(`{
		"run_id":"run-123",
		"enclave_backend":"awf-enclave",
		"allowed_repositories":["github/gh-aw"],
		"tool_policy":"github-repository-read-v1",
		"allowed_schema_hashes":["sha256:abc"],
		"max_identity_ttl":120,
		"expires_at":"2030-01-01T00:00:00Z"
	}`), &wire))

	envelope, err := wire.ToEnvelope()
	require.NoError(t, err)
	assert.Equal(t, 120*time.Second, envelope.MaxIdentityTTL)
}

func TestEnvelopeWire_RejectsInvalidSeconds(t *testing.T) {
	tests := []struct {
		name    string
		seconds int64
	}{
		{name: "zero", seconds: 0},
		{name: "negative", seconds: -1},
		{name: "overflow", seconds: int64(^uint64(0)>>1)/int64(time.Second) + 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (EnvelopeWire{MaxIdentityTTLSeconds: test.seconds}).ToEnvelope()
			assert.Error(t, err)
		})
	}
}

func validEnvelope() *Envelope {
	return &Envelope{
		RunID:               "run-123",
		EnclaveBackend:      "awf-enclave",
		AllowedRepositories: []string{"github/gh-aw", "github/gh-aw-firewall"},
		ToolPolicy:          ToolPolicyGitHubRepositoryReadV1,
		AllowedSchemaHashes: []string{"sha256:abc"},
		MaxIdentityTTL:      5 * time.Minute,
		ExpiresAt:           time.Now().Add(time.Hour),
	}
}

func TestEnvelopeValidate(t *testing.T) {
	require.NoError(t, validEnvelope().Validate())

	t.Run("nil envelope rejected", func(t *testing.T) {
		var e *Envelope
		assert.Error(t, e.Validate())
	})

	// mutations that are individually expected to make an otherwise-valid
	// envelope invalid, keyed by the specific fmt.Errorf message Validate
	// returns for that invariant.
	tests := []struct {
		name       string
		mutate     func(e *Envelope)
		wantErrMsg string
	}{
		{
			name:       "missing run id",
			mutate:     func(e *Envelope) { e.RunID = "" },
			wantErrMsg: "envelope run id is required",
		},
		{
			name:       "missing enclave backend",
			mutate:     func(e *Envelope) { e.EnclaveBackend = "" },
			wantErrMsg: "envelope enclave backend is required",
		},
		{
			name:       "zero expiry rejected",
			mutate:     func(e *Envelope) { e.ExpiresAt = time.Time{} },
			wantErrMsg: "envelope expiry is required",
		},
		{
			name:       "unsupported tool policy rejected",
			mutate:     func(e *Envelope) { e.ToolPolicy = "github-repository-write-v1" },
			wantErrMsg: "unsupported envelope tool policy",
		},
		{
			name:       "zero ttl rejected",
			mutate:     func(e *Envelope) { e.MaxIdentityTTL = 0 },
			wantErrMsg: "envelope max identity ttl must be positive",
		},
		{
			name:       "no repositories and no owners rejected",
			mutate:     func(e *Envelope) { e.AllowedRepositories = nil },
			wantErrMsg: "envelope must admit at least one repository or owner",
		},
		{
			name:       "no schema hashes and no dynamic bound rejected",
			mutate:     func(e *Envelope) { e.AllowedSchemaHashes = nil },
			wantErrMsg: "envelope must admit at least one schema hash",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEnvelope()
			tt.mutate(e)
			assert.ErrorContains(t, e.Validate(), tt.wantErrMsg)
		})
	}

	// these cases additionally assert that the rejected selector value is
	// never leaked unredacted into the returned error message.
	redactionTests := []struct {
		name   string
		mutate func(e *Envelope)
		leaked string
	}{
		{
			name:   "noncanonical repository rejected",
			mutate: func(e *Envelope) { e.AllowedRepositories = []string{"GitHub/gh-aw"} },
			leaked: "GitHub/gh-aw",
		},
		{
			name:   "duplicate repository rejected",
			mutate: func(e *Envelope) { e.AllowedRepositories = []string{"github/gh-aw", "github/gh-aw"} },
			leaked: "github/gh-aw",
		},
		{
			name:   "noncanonical owner rejected",
			mutate: func(e *Envelope) { e.AllowedOwners = []string{"GitHub"} },
			leaked: "GitHub",
		},
		{
			name:   "duplicate owner rejected",
			mutate: func(e *Envelope) { e.AllowedOwners = []string{"github", "github"} },
			leaked: "github",
		},
	}
	for _, tt := range redactionTests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEnvelope()
			tt.mutate(e)
			err := e.Validate()
			require.Error(t, err)
			assert.NotContains(t, err.Error(), tt.leaked, "validation error must not leak unredacted selector")
		})
	}

	t.Run("owner-only envelope with no repositories is valid", func(t *testing.T) {
		e := validEnvelope()
		e.AllowedRepositories = nil
		e.AllowedOwners = []string{"github"}
		assert.NoError(t, e.Validate())
	})

	t.Run("dynamic schema mode with a positive bound is valid", func(t *testing.T) {
		e := validEnvelope()
		e.AllowedSchemaHashes = nil
		e.MaxDynamicSchemaHashes = 1
		assert.NoError(t, e.Validate())
	})
}

func TestEnvelopeAllowsRepository(t *testing.T) {
	tests := []struct {
		name    string
		envMod  func(e *Envelope)
		repo    string
		want    bool
		wantMsg string
	}{
		{name: "exact repository match allowed", repo: "github/gh-aw", want: true},
		{name: "case-sensitive comparison rejects mismatch", repo: "github/GH-AW", want: false},
		{name: "unrelated repository rejected", repo: "other/other", want: false},
		{
			name:   "owner-scoped: exact repository under owner allowed",
			envMod: func(e *Envelope) { e.AllowedRepositories = nil; e.AllowedOwners = []string{"github"} },
			repo:   "github/gh-aw",
			want:   true,
		},
		{
			name:   "owner-scoped: any repository under the owner allowed",
			envMod: func(e *Envelope) { e.AllowedRepositories = nil; e.AllowedOwners = []string{"github"} },
			repo:   "github/any-repo-under-the-owner",
			want:   true,
		},
		{
			name:    "owner-scoped: sibling owner not admitted",
			envMod:  func(e *Envelope) { e.AllowedRepositories = nil; e.AllowedOwners = []string{"github"} },
			repo:    "other-owner/private-repo",
			want:    false,
			wantMsg: "a sibling owner must not be admitted",
		},
		{
			name:    "owner-scoped: owner comparison is exact-byte, not case-insensitive",
			envMod:  func(e *Envelope) { e.AllowedRepositories = nil; e.AllowedOwners = []string{"github"} },
			repo:    "GitHub/gh-aw",
			want:    false,
			wantMsg: "owner comparison is exact-byte, not case-insensitive",
		},
		{
			name:   "owner-scoped: non-canonical selector never admitted via owner path",
			envMod: func(e *Envelope) { e.AllowedRepositories = nil; e.AllowedOwners = []string{"github"} },
			repo:   "not-a-canonical-selector",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := validEnvelope()
			if tt.envMod != nil {
				tt.envMod(e)
			}
			got := e.AllowsRepository(tt.repo)
			if tt.wantMsg != "" {
				assert.Equal(t, tt.want, got, tt.wantMsg)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestEnvelopeAllowsSchemaHash(t *testing.T) {
	e := validEnvelope()
	assert.True(t, e.AllowsSchemaHash("sha256:abc"))
	assert.False(t, e.AllowsSchemaHash("sha256:def"))
}

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/guard"
	"github.com/stretchr/testify/assert"
)

// TestResolveRequestIdentity_AuthEnabled_IgnoresXAgentID is the core anti-spoofing
// test: when authentication is enabled, the authenticated identity is derived ONLY
// from the Authorization header. A client-supplied X-Agent-ID header must never be
// able to override the authenticated principal.
func TestResolveRequestIdentity_AuthEnabled_IgnoresXAgentID(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		xAgentID   string
		want       string
	}{
		{
			name:       "X-Agent-ID cannot override Authorization identity",
			authHeader: "victim-agent",
			xAgentID:   "attacker-agent",
			want:       "victim-agent",
		},
		{
			name:       "X-Agent-ID alone is ignored when auth enabled",
			authHeader: "",
			xAgentID:   "attacker-agent",
			want:       "",
		},
		{
			name:       "plain Authorization identity is extracted",
			authHeader: "real-agent",
			xAgentID:   "attacker-agent",
			want:       "real-agent",
		},
		{
			name:       "malformed Authorization rejected",
			authHeader: "bad\x00id",
			want:       "",
		},
		{
			name:       "blank Authorization rejected",
			authHeader: "",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.xAgentID != "" {
				req.Header.Set("X-Agent-ID", tt.xAgentID)
			}
			got := resolveRequestIdentity(req, true)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestResolveRequestIdentity_AuthDisabled_LegacyBehavior verifies that when auth is
// disabled there is no authenticated identity to protect, so the legacy
// X-Agent-ID/Authorization resolution is preserved (backward compatible).
func TestResolveRequestIdentity_AuthDisabled_LegacyBehavior(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "auth-agent")
	req.Header.Set("X-Agent-ID", "legacy-agent")

	// Legacy behavior: X-Agent-ID takes precedence when auth is disabled.
	assert.Equal(t, "legacy-agent", resolveRequestIdentity(req, false))
}

// TestSetupSessionCallback_AuthEnabled_SpoofingRejected verifies that the session
// established for a request uses the authenticated identity and that a spoofed
// X-Agent-ID does not change the session identity used for context/DIFC keying.
func TestSetupSessionCallback_AuthEnabled_SpoofingRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "victim-agent")
	req.Header.Set("X-Agent-ID", "attacker-agent")

	sessionID, ok := setupSessionCallback(req, "", true)
	assert.True(t, ok)
	assert.Equal(t, "victim-agent", sessionID, "authenticated identity must come from Authorization only")

	ctxSessionID := req.Context().Value(SessionIDContextKey)
	assert.Equal(t, "victim-agent", ctxSessionID, "context/DIFC identity must be the authenticated principal")

	// The guard/DIFC agent identity must also be keyed to the authenticated
	// principal, not the spoofed X-Agent-ID header.
	assert.Equal(t, "victim-agent", guard.GetAgentIDFromContext(req.Context()),
		"guard/DIFC identity must be the authenticated principal")
}

// TestSetupSessionCallback_AuthEnabled_BlankRejected verifies a blank Authorization
// header is rejected when auth is enabled even if an X-Agent-ID is supplied.
func TestSetupSessionCallback_AuthEnabled_BlankRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("X-Agent-ID", "attacker-agent")

	_, ok := setupSessionCallback(req, "", true)
	assert.False(t, ok, "blank Authorization must be rejected when auth is enabled")
}

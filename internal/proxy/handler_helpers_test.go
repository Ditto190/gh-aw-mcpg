package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteDIFCForbidden verifies that writeDIFCForbidden writes a 403 JSON
// response with the "difc_forbidden" code and the supplied message.
func TestWriteDIFCForbidden(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "simple message",
			message: "secrecy violation: agent not authorized",
		},
		{
			name:    "integrity message",
			message: "integrity violation for write to resource 'github_api': the agent's integrity level is insufficient",
		},
		{
			name:    "empty message",
			message: "",
		},
		{
			name:    "message with special characters",
			message: "violation: agent has tag [private:org/repo], resource requires none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeDIFCForbidden(w, tt.message)

			assert.Equal(t, http.StatusForbidden, w.Code, "expected HTTP 403 Forbidden")
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"), "expected JSON content type")

			var body map[string]string
			err := json.Unmarshal(w.Body.Bytes(), &body)
			require.NoError(t, err, "response body should be valid JSON")
			assert.Equal(t, "difc_forbidden", body["error"], "expected difc_forbidden error code")
			assert.Equal(t, tt.message, body["message"], "expected message to be preserved")
		})
	}
}

// TestIsMetadataPassthroughPath verifies the allowlist of safe metadata endpoints
// that bypass DIFC labeling.
func TestIsMetadataPassthroughPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Paths in the allowlist
		{path: "/meta", want: true},
		{path: "/rate_limit", want: true},
		{path: "/octocat", want: true},
		{path: "/zen", want: true},
		{path: "/versions", want: true},

		// Paths not in the allowlist
		{path: "/repos/owner/repo", want: false},
		{path: "/repos/owner/repo/issues", want: false},
		{path: "/graphql", want: false},
		{path: "/user", want: false},
		{path: "/orgs/myorg", want: false},
		{path: "/health", want: false},
		{path: "/reflect", want: false},
		{path: "", want: false},

		// Prefix matches should NOT work (exact match required)
		{path: "/meta/anything", want: false},
		{path: "/rate_limit/core", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isMetadataPassthroughPath(tt.path)
			assert.Equal(t, tt.want, got, "isMetadataPassthroughPath(%q)", tt.path)
		})
	}
}

// TestIsMetadataPassthroughPath_CompleteAllowlist verifies that all documented
// metadata paths are present in the allowlist and no undocumented paths are added.
func TestIsMetadataPassthroughPath_CompleteAllowlist(t *testing.T) {
	// These are the five documented safe metadata endpoints per the proxy handler.
	// Any change to this list requires review for DIFC security implications.
	expectedAllowlist := []string{
		"/meta",
		"/rate_limit",
		"/octocat",
		"/zen",
		"/versions",
	}

	for _, path := range expectedAllowlist {
		assert.True(t, isMetadataPassthroughPath(path),
			"expected %q to be in the metadata passthrough allowlist", path)
	}

	// The allowlist map should contain exactly these five paths.
	assert.Equal(t, len(expectedAllowlist), len(metadataPassthrough),
		"metadataPassthrough allowlist size changed: review DIFC security implications")
}

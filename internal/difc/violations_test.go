package difc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestViolationError_Error_Secrecy tests the secrecy violation error message formatting.
func TestViolationError_Error_Secrecy(t *testing.T) {
	tests := []struct {
		name      string
		err       *ViolationError
		wantParts []string
	}{
		{
			name: "secrecy violation with extra tags",
			err: &ViolationError{
				Type:      SecrecyViolation,
				Resource:  "my-repo",
				IsWrite:   false,
				ExtraTags: []Tag{"private:org/repo"},
			},
			wantParts: []string{"Secrecy violation", "my-repo", "not authorized"},
		},
		{
			name: "secrecy violation with no extra tags",
			err: &ViolationError{
				Type:     SecrecyViolation,
				Resource: "some-resource",
				IsWrite:  false,
			},
			wantParts: []string{"Secrecy violation", "some-resource"},
		},
		{
			name: "secrecy violation with multiple extra tags",
			err: &ViolationError{
				Type:      SecrecyViolation,
				Resource:  "sensitive-data",
				ExtraTags: []Tag{"private:org/repo", "private"},
			},
			wantParts: []string{"Secrecy violation", "sensitive-data"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			require.NotEmpty(t, msg)
			for _, part := range tt.wantParts {
				assert.Contains(t, msg, part)
			}
		})
	}
}

// TestViolationError_Error_Integrity tests integrity violation error message formatting.
func TestViolationError_Error_Integrity(t *testing.T) {
	tests := []struct {
		name        string
		err         *ViolationError
		wantParts   []string
		wantAbsent  []string
	}{
		{
			name: "integrity write violation with missing tags",
			err: &ViolationError{
				Type:        IntegrityViolation,
				Resource:    "protected-repo",
				IsWrite:     true,
				MissingTags: []Tag{"approved:all"},
			},
			wantParts: []string{"Integrity violation", "write", "protected-repo", "insufficient"},
		},
		{
			name: "integrity read violation with missing tags",
			err: &ViolationError{
				Type:        IntegrityViolation,
				Resource:    "high-integrity-resource",
				IsWrite:     false,
				MissingTags: []Tag{"merged:all"},
			},
			wantParts: []string{"Integrity violation", "read", "high-integrity-resource"},
		},
		{
			name: "integrity write violation with no missing tags",
			err: &ViolationError{
				Type:     IntegrityViolation,
				Resource: "some-resource",
				IsWrite:  true,
			},
			wantParts: []string{"Integrity violation", "write", "some-resource"},
		},
		{
			name: "integrity read violation with no missing tags",
			err: &ViolationError{
				Type:    IntegrityViolation,
				Resource: "read-resource",
				IsWrite: false,
			},
			wantParts: []string{"Integrity violation", "read", "read-resource"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			require.NotEmpty(t, msg)
			for _, part := range tt.wantParts {
				assert.Contains(t, msg, part, "expected %q in message %q", part, msg)
			}
			for _, absent := range tt.wantAbsent {
				assert.NotContains(t, msg, absent)
			}
		})
	}
}

// TestViolationError_Detailed tests that Detailed() extends Error() with extra context.
func TestViolationError_Detailed(t *testing.T) {
	err := &ViolationError{
		Type:         SecrecyViolation,
		Resource:     "test-resource",
		ExtraTags:    []Tag{"private:org/repo"},
		AgentTags:    []Tag{"private:org/repo", "public"},
		ResourceTags: []Tag{"public"},
	}

	base := err.Error()
	detailed := err.Detailed()

	assert.Contains(t, detailed, base, "Detailed should contain base error message")
	assert.Contains(t, detailed, "Agent", "Detailed should include agent tags context")
	assert.Contains(t, detailed, "Resource", "Detailed should include resource tags context")
	// Detailed should be longer than Error
	assert.Greater(t, len(detailed), len(base))
}

// TestViolationError_Detailed_Integrity tests Detailed() for integrity violations.
func TestViolationError_Detailed_Integrity(t *testing.T) {
	err := &ViolationError{
		Type:         IntegrityViolation,
		Resource:     "protected",
		IsWrite:      true,
		MissingTags:  []Tag{"approved:all"},
		AgentTags:    []Tag{"unapproved:all"},
		ResourceTags: []Tag{"approved:all"},
	}

	detailed := err.Detailed()
	assert.Contains(t, detailed, "Integrity violation")
	assert.Contains(t, detailed, "protected")
	// Should contain context about agent and resource tags
	assert.Contains(t, detailed, "Agent")
	assert.Contains(t, detailed, "Resource")
}

// TestFormatViolationError_AllowDecision tests that allowed decisions return nil.
func TestFormatViolationError_AllowDecision(t *testing.T) {
	result := &EvaluationResult{
		Decision: AccessAllow,
		Reason:   "access allowed",
	}
	agentSecrecy := NewSecrecyLabel()
	agentIntegrity := NewIntegrityLabel()
	resource := NewLabeledResource("test resource")

	err := FormatViolationError(result, agentSecrecy, agentIntegrity, resource)
	assert.NoError(t, err, "AllowDecision should return nil error")
}

// TestFormatViolationError_AllowWithPropagateDecision tests that propagate decisions return nil.
func TestFormatViolationError_AllowWithPropagateDecision(t *testing.T) {
	result := &EvaluationResult{
		Decision: AccessAllowWithPropagate,
		Reason:   "access allowed with propagation",
	}
	agentSecrecy := NewSecrecyLabel()
	agentIntegrity := NewIntegrityLabel()
	resource := NewLabeledResource("test resource")

	err := FormatViolationError(result, agentSecrecy, agentIntegrity, resource)
	assert.NoError(t, err, "AllowWithPropagate should return nil error")
}

// TestFormatViolationError_DenyWithSecrecyTags tests denial with required secrecy tag additions.
func TestFormatViolationError_DenyWithSecrecyTags(t *testing.T) {
	result := &EvaluationResult{
		Decision:     AccessDeny,
		Reason:       "agent must accept secrecy tags to read private data",
		SecrecyToAdd: []Tag{"private:org/repo"},
	}
	agentSecrecy := NewSecrecyLabel()
	agentIntegrity := NewIntegrityLabel()
	resource := NewLabeledResource("private repository")
	resource.Secrecy = *NewSecrecyLabel(Tag("private:org/repo"))

	err := FormatViolationError(result, agentSecrecy, agentIntegrity, resource)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "DIFC Violation")
	assert.Contains(t, msg, "agent must accept secrecy tags")
	assert.Contains(t, msg, "Required Action")
	assert.Contains(t, msg, "secrecy tags")
	assert.Contains(t, msg, "public resources")
}

// TestFormatViolationError_DenyWithIntegrityTags tests denial with required integrity tag drops.
func TestFormatViolationError_DenyWithIntegrityTags(t *testing.T) {
	result := &EvaluationResult{
		Decision:        AccessDeny,
		Reason:          "agent integrity too low for this write",
		IntegrityToDrop: []Tag{"approved:all"},
	}
	agentSecrecy := NewSecrecyLabel()
	agentIntegrity := NewIntegrityLabel(Tag("approved:all"))
	resource := NewLabeledResource("high-integrity target")

	err := FormatViolationError(result, agentSecrecy, agentIntegrity, resource)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "DIFC Violation")
	assert.Contains(t, msg, "agent integrity too low")
	assert.Contains(t, msg, "Required Action")
	assert.Contains(t, msg, "integrity tags")
}

// TestFormatViolationError_DenyWithBothTagChanges tests denial requiring both secrecy and integrity changes.
func TestFormatViolationError_DenyWithBothTagChanges(t *testing.T) {
	result := &EvaluationResult{
		Decision:        AccessDeny,
		Reason:          "multiple violations",
		SecrecyToAdd:    []Tag{"private:org/repo"},
		IntegrityToDrop: []Tag{"approved:all"},
	}
	agentSecrecy := NewSecrecyLabel()
	agentIntegrity := NewIntegrityLabel(Tag("approved:all"))
	resource := NewLabeledResource("complex resource")

	err := FormatViolationError(result, agentSecrecy, agentIntegrity, resource)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "DIFC Violation")
	assert.Contains(t, msg, "multiple violations")
	assert.Contains(t, msg, "Current Agent Labels")
	assert.Contains(t, msg, "Resource Requirements")
}

// TestFormatViolationError_DenyNoTagChanges tests denial message with no tag changes specified.
func TestFormatViolationError_DenyNoTagChanges(t *testing.T) {
	result := &EvaluationResult{
		Decision: AccessDeny,
		Reason:   "access denied: insufficient permissions",
	}
	agentSecrecy := NewSecrecyLabel()
	agentIntegrity := NewIntegrityLabel()
	resource := NewLabeledResource("restricted resource")

	err := FormatViolationError(result, agentSecrecy, agentIntegrity, resource)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "DIFC Violation")
	assert.Contains(t, msg, "access denied")
	assert.Contains(t, msg, "Current Agent Labels")
	assert.Contains(t, msg, "Resource Requirements")
}

// TestFormatIntegrityLevel tests the formatIntegrityLevel helper function.
func TestFormatIntegrityLevel(t *testing.T) {
	tests := []struct {
		name string
		tags []Tag
		want string
	}{
		{
			name: "empty tags returns none",
			tags: nil,
			want: "none",
		},
		{
			name: "empty slice returns none",
			tags: []Tag{},
			want: "none",
		},
		{
			name: "merged tag wins over all others",
			tags: []Tag{"merged:all"},
			want: `"merged"`,
		},
		{
			name: "merged tag immediately returns regardless of other tags",
			tags: []Tag{"unapproved:all", "approved:all", "merged:all"},
			want: `"merged"`,
		},
		{
			name: "approved tag is higher than unapproved",
			tags: []Tag{"approved:all"},
			want: `"approved"`,
		},
		{
			name: "approved wins over unapproved when both present",
			tags: []Tag{"unapproved:all", "approved:all"},
			want: `"approved"`,
		},
		{
			name: "unapproved tag when alone",
			tags: []Tag{"unapproved:all"},
			want: `"unapproved"`,
		},
		{
			name: "unknown tag uses default format",
			tags: []Tag{"custom:scope"},
			want: "[custom:scope]",
		},
		{
			name: "tag without scope suffix (merged)",
			tags: []Tag{"merged"},
			want: `"merged"`,
		},
		{
			name: "tag without scope suffix (approved)",
			tags: []Tag{"approved"},
			want: `"approved"`,
		},
		{
			name: "tag without scope suffix (unapproved)",
			tags: []Tag{"unapproved"},
			want: `"unapproved"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatIntegrityLevel(tt.tags)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFormatSecrecyLevel tests the formatSecrecyLevel helper function.
func TestFormatSecrecyLevel(t *testing.T) {
	tests := []struct {
		name string
		tags []Tag
		want string
	}{
		{
			name: "empty tags returns public",
			tags: nil,
			want: "public",
		},
		{
			name: "empty slice returns public",
			tags: []Tag{},
			want: "public",
		},
		{
			name: "private tag without scope",
			tags: []Tag{"private"},
			want: "private",
		},
		{
			name: "private tag with scope",
			tags: []Tag{"private:org/repo"},
			want: "private (org/repo)",
		},
		{
			name: "private tag with longer scope wins",
			tags: []Tag{"private:org", "private:org/repo"},
			want: "private (org/repo)",
		},
		{
			name: "private tag with shorter scope loses to longer",
			tags: []Tag{"private:org/repo", "private:org"},
			want: "private (org/repo)",
		},
		{
			name: "private scope wins over plain private",
			tags: []Tag{"private", "private:org/repo"},
			want: "private (org/repo)",
		},
		{
			name: "private: with empty scope ignored",
			tags: []Tag{"private:"},
			want: "[private:]",
		},
		{
			name: "unknown tags use default format",
			tags: []Tag{"custom-tag"},
			want: "[custom-tag]",
		},
		{
			name: "multiple private scopes picks longest",
			tags: []Tag{"private:a", "private:a/b/c", "private:a/b"},
			want: "private (a/b/c)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSecrecyLevel(tt.tags)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFormatSecrecyLevel_PrivateColonEmptyScope tests edge case where "private:"
// has an empty scope component and falls through to default formatting.
func TestFormatSecrecyLevel_PrivateColonEmptyScope(t *testing.T) {
	tags := []Tag{"private:"}
	got := formatSecrecyLevel(tags)
	// "private:" has empty scope, does not set hasPrivate → falls to fmt.Sprintf default
	assert.NotContains(t, got, "()", "should not produce empty parens")
	assert.Equal(t, "[private:]", got, "empty scope falls through to default fmt.Sprintf")
}

// TestViolationError_SecrecyInErrorMessage tests the secrecy level description appears in error.
func TestViolationError_SecrecyInErrorMessage(t *testing.T) {
	err := &ViolationError{
		Type:      SecrecyViolation,
		Resource:  "my-repo",
		ExtraTags: []Tag{"private:org/repo"},
	}
	msg := err.Error()
	// Should include human-readable secrecy level
	assert.True(t, strings.Contains(msg, "private") || strings.Contains(msg, "org/repo"),
		"error message should describe the secrecy level: %s", msg)
}

// TestViolationError_IntegrityInErrorMessage tests the integrity level description appears in error.
func TestViolationError_IntegrityInErrorMessage(t *testing.T) {
	err := &ViolationError{
		Type:        IntegrityViolation,
		Resource:    "protected-resource",
		IsWrite:     true,
		MissingTags: []Tag{"approved:all"},
	}
	msg := err.Error()
	// Should include human-readable integrity level
	assert.Contains(t, msg, "approved", "error message should describe the required integrity level: %s", msg)
}

// TestFormatViolationError_MessageContainsAgentAndResourceLabels verifies the message includes
// agent and resource label states for debugging purposes.
func TestFormatViolationError_MessageContainsAgentAndResourceLabels(t *testing.T) {
	result := &EvaluationResult{
		Decision: AccessDeny,
		Reason:   "test denial",
	}
	agentSecrecy := NewSecrecyLabel(Tag("private"))
	agentIntegrity := NewIntegrityLabel(Tag("approved:all"))
	resource := NewLabeledResource("test")
	resource.Secrecy = *NewSecrecyLabel(Tag("public"))

	err := FormatViolationError(result, agentSecrecy, agentIntegrity, resource)
	require.Error(t, err)
	msg := err.Error()

	assert.Contains(t, msg, "Current Agent Labels", "should show agent label context")
	assert.Contains(t, msg, "Resource Requirements", "should show resource requirements")
	assert.Contains(t, msg, "Secrecy", "should mention secrecy labels")
	assert.Contains(t, msg, "Integrity", "should mention integrity labels")
}

package difc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFormatIntegrityLevel tests all branches of formatIntegrityLevel.
func TestFormatIntegrityLevel(t *testing.T) {
	tests := []struct {
		name string
		tags []Tag
		want string
	}{
		{
			name: "empty tags returns none",
			tags: []Tag{},
			want: "none",
		},
		{
			name: "nil tags returns none",
			tags: nil,
			want: "none",
		},
		{
			name: "merged tag returns merged immediately",
			tags: []Tag{"merged"},
			want: `"merged"`,
		},
		{
			name: "merged with scope suffix returns merged",
			tags: []Tag{"merged:org/repo"},
			want: `"merged"`,
		},
		{
			name: "merged overrides approved in same list",
			tags: []Tag{"approved", "merged"},
			want: `"merged"`,
		},
		{
			name: "approved tag returns approved",
			tags: []Tag{"approved"},
			want: `"approved"`,
		},
		{
			name: "approved with scope suffix strips scope",
			tags: []Tag{"approved:all"},
			want: `"approved"`,
		},
		{
			name: "approved with repo scope strips scope",
			tags: []Tag{"approved:org/repo"},
			want: `"approved"`,
		},
		{
			name: "approved overrides unapproved",
			tags: []Tag{"unapproved", "approved"},
			want: `"approved"`,
		},
		{
			name: "unapproved tag returns unapproved",
			tags: []Tag{"unapproved"},
			want: `"unapproved"`,
		},
		{
			name: "unapproved with scope suffix strips scope",
			tags: []Tag{"unapproved:all"},
			want: `"unapproved"`,
		},
		{
			name: "unapproved does not override already-set approved",
			tags: []Tag{"approved", "unapproved"},
			want: `"approved"`,
		},
		{
			name: "unknown tag returns fmt.Sprintf representation",
			tags: []Tag{"custom-tag"},
			want: "[custom-tag]",
		},
		{
			name: "multiple unknown tags returns formatted list",
			tags: []Tag{"tag1", "tag2"},
			want: "[tag1 tag2]",
		},
		{
			name: "colon-prefixed unknown tag does not match known levels",
			// ":approved" has idx==0, so idx > 0 is false; treated as unknown
			tags: []Tag{":approved"},
			want: "[:approved]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatIntegrityLevel(tt.tags)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFormatSecrecyLevel tests all branches of formatSecrecyLevel.
func TestFormatSecrecyLevel(t *testing.T) {
	tests := []struct {
		name string
		tags []Tag
		want string
	}{
		{
			name: "empty tags returns public",
			tags: []Tag{},
			want: "public",
		},
		{
			name: "nil tags returns public",
			tags: nil,
			want: "public",
		},
		{
			name: "scoped private tag returns private with scope",
			tags: []Tag{"private:org/repo"},
			want: "private (org/repo)",
		},
		{
			name: "multiple scoped private tags uses longest scope",
			tags: []Tag{"private:org", "private:org/repo"},
			want: "private (org/repo)",
		},
		{
			name: "longest scope wins regardless of order",
			tags: []Tag{"private:org/repo", "private:org"},
			want: "private (org/repo)",
		},
		{
			name: "bare private tag returns private",
			tags: []Tag{"private"},
			want: "private",
		},
		{
			name: "scoped private takes priority over bare private",
			tags: []Tag{"private", "private:org/repo"},
			want: "private (org/repo)",
		},
		{
			name: "private: with empty scope does not set bestScope",
			// "private:" has scope="" after TrimPrefix, len("") is not > len(""), so skipped
			tags: []Tag{"private:"},
			// bestScope stays "", hasPrivate stays false, falls to fmt.Sprintf
			want: "[private:]",
		},
		{
			name: "unknown tag returns fmt.Sprintf representation",
			tags: []Tag{"internal"},
			want: "[internal]",
		},
		{
			name: "multiple unknown tags returns formatted list",
			tags: []Tag{"confidential", "restricted"},
			want: "[confidential restricted]",
		},
		{
			name: "mix of unknown tags and bare private returns private",
			tags: []Tag{"other", "private"},
			want: "private",
		},
		{
			name: "mix of unknown tags and scoped private returns scoped",
			tags: []Tag{"other", "private:myorg"},
			want: "private (myorg)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSecrecyLevel(tt.tags)
			assert.Equal(t, tt.want, got)
		})
	}
}

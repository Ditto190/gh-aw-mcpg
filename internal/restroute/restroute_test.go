package restroute

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripQuery(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "strips query string",
			path: "/repos/github/gh-aw/issues?page=1",
			want: "/repos/github/gh-aw/issues",
		},
		{
			name: "no query string returns path unchanged",
			path: "/repos/github/gh-aw/issues",
			want: "/repos/github/gh-aw/issues",
		},
		{
			name: "empty path returns empty string",
			path: "",
			want: "",
		},
		{
			name: "query marker only",
			path: "?",
			want: "",
		},
		{
			name: "multiple query markers strips at first occurrence",
			path: "/issues?page=1?extra=2",
			want: "/issues",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, StripQuery(tt.path))
		})
	}
}

func TestMatch(t *testing.T) {
	pattern := regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues$`)

	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "matching path returns full match and submatches",
			path: "/repos/github/gh-aw/issues",
			want: []string{"/repos/github/gh-aw/issues", "github", "gh-aw"},
		},
		{
			name: "path with query string does not match",
			path: "/repos/github/gh-aw/issues?page=1",
			want: nil,
		},
		{
			name: "non-matching path returns nil",
			path: "/repos/github/gh-aw/pulls",
			want: nil,
		},
		{
			name: "empty path returns nil",
			path: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Match(tt.path, pattern))
		})
	}
}

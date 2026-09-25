package reposelector

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCanonicalOwner(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		want  bool
	}{
		{"single char", "a", true},
		{"digits", "123456", true},
		{"hyphen", "my-org", true},
		{"exactly 39 chars", strings.Repeat("a", 39), true},
		{"exactly 40 chars", strings.Repeat("a", 40), false},
		{"empty string", "", false},
		{"underscore", "my_org", false},
		{"leading hyphen", "-org", false},
		{"uppercase", "MyOrg", false},
		{"dot", "my.org", false},
		{"non-ASCII", "gi\u2122thub", false},
		{"slash", "my/org", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsCanonicalOwner(tt.owner), "IsCanonicalOwner(%q)", tt.owner)
		})
	}
}

func TestIsLegacyOwner(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		want  bool
	}{
		{"underscore is allowed", "my_org", true},
		{"hyphen is allowed", "my-org", true},
		{"exactly 39 chars", strings.Repeat("a", 39), true},
		{"exactly 40 chars", strings.Repeat("a", 40), false},
		{"empty string", "", false},
		{"leading underscore", "_org", false},
		{"uppercase", "MyOrg", false},
		{"dot", "my.org", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsLegacyOwner(tt.owner), "IsLegacyOwner(%q)", tt.owner)
		})
	}
}

func TestIsCanonicalRepoName(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		want     bool
	}{
		{"simple", "gh-aw", true},
		{"underscore", "my_repo", true},
		{"dot", "my.repo", true},
		{"leading dot", ".github", true},
		{"exactly 100 chars", strings.Repeat("a", 100), true},
		{"exactly 101 chars", strings.Repeat("a", 101), false},
		{"empty string", "", false},
		{"single dot", ".", false},
		{"double dot", "..", false},
		{"contains double dot", "foo..bar", false},
		{"uppercase", "MyRepo", false},
		{"space", "my repo", false},
		{"slash", "my/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsCanonicalRepoName(tt.repoName), "IsCanonicalRepoName(%q)", tt.repoName)
		})
	}
}

func TestIsCanonicalRepositorySelector(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		want     bool
	}{
		{"canonical selector", "github/gh-aw", true},
		{"dotted repo name", "github/gh.aw", true},
		{"underscore repo name", "github/gh_aw", true},
		{"underscore owner", "my_org/repo", false},
		{"uppercase owner", "GitHub/gh-aw", false},
		{"no slash", "github", false},
		{"three segments", "github/gh-aw/extra", false},
		{"empty owner", "/repo", false},
		{"empty repo", "github/", false},
		{"traversal repo", "github/..", false},
		{"embedded traversal", "github/foo..bar", false},
		{"non-ASCII owner", "gi™thub/gh-aw", false},
		{"non-ASCII repo", "github/gh™aw", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsCanonicalRepositorySelector(tt.selector), "IsCanonicalRepositorySelector(%q)", tt.selector)
		})
	}
}

func TestIsLegacyRepositorySelector(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		want     bool
	}{
		{"canonical selector", "github/gh-aw", true},
		{"underscore owner is allowed", "my_org/repo", true},
		{"underscore repo", "github/gh_aw", true},
		{"dot in owner is rejected", "my.org/repo", false},
		{"uppercase", "GitHub/gh-aw", false},
		{"traversal repo", "github/a..b", false},
		{"three segments", "github/gh-aw/extra", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsLegacyRepositorySelector(tt.selector), "IsLegacyRepositorySelector(%q)", tt.selector)
		})
	}
}

func TestIsTraversalRepoName(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
		want     bool
	}{
		{"single dot", ".", true},
		{"double dot", "..", true},
		{"embedded double dot", "foo..bar", true},
		{"trailing double dot", "foo..", true},
		{"ordinary name", "gh-aw", false},
		{"single dot inside name", "gh.aw", false},
		{"leading dot", ".github", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsTraversalRepoName(tt.repoName), "IsTraversalRepoName(%q)", tt.repoName)
		})
	}
}

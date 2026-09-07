package restroute

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripQuery(t *testing.T) {
	assert.Equal(t, "/repos/github/gh-aw/issues", StripQuery("/repos/github/gh-aw/issues?page=1"))
}

func TestMatch(t *testing.T) {
	pattern := regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues$`)

	assert.Equal(t, []string{"/repos/github/gh-aw/issues", "github", "gh-aw"}, Match("/repos/github/gh-aw/issues", pattern))
	assert.Nil(t, Match("/repos/github/gh-aw/issues?page=1", pattern))
}

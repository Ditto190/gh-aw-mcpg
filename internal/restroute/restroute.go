// Package restroute provides shared REST path-matching helpers.
package restroute

import (
	"regexp"
	"strings"

	"github.com/github/gh-aw-mcpg/internal/logger"
)

var log = logger.ForFile()

// StripQuery removes the query string from a REST path.
func StripQuery(path string) string {
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		if log.Enabled() {
			log.Printf("StripQuery: stripped query string from path (original len=%d, stripped len=%d)", len(path), idx)
		}
		return path[:idx]
	}
	return path
}

// Match returns the regex submatches for path.
func Match(path string, pattern *regexp.Regexp) []string {
	matches := pattern.FindStringSubmatch(path)
	if matches == nil {
		log.Printf("Match: no match for pattern %q against path", pattern.String())
	} else {
		log.Printf("Match: pattern %q matched path with %d submatches", pattern.String(), len(matches))
	}
	return matches
}

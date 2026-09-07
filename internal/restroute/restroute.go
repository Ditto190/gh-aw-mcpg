// Package restroute provides shared REST path-matching helpers.
package restroute

import (
	"regexp"
	"strings"
)

// StripQuery removes the query string from a REST path.
func StripQuery(path string) string {
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		return path[:idx]
	}
	return path
}

// Match returns the regex submatches for path.
func Match(path string, pattern *regexp.Regexp) []string {
	return pattern.FindStringSubmatch(path)
}

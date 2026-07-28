// Redact functions mask a known secret value for safe display in logs.
// Unlike the Sanitize* functions (which scan arbitrary strings for secret patterns),
// Redact* functions operate on a value that is already known to be sensitive and
// truncate or strip it so only a safe hint remains.
package sanitize

import "net/url"

// RedactSecret returns a sanitized version of the input string for safe logging.
// It shows only the first 4 characters followed by "..." to prevent exposing sensitive data.
// For strings with 4 or fewer characters, it returns only "...".
// For empty strings, it returns an empty string.
func RedactSecret(input string) string {
	if len(input) == 0 {
		return ""
	}
	const prefixLen = 4
	if len(input) <= prefixLen {
		return "..."
	}
	return input[:prefixLen] + "..."
}

// RedactSecretMap returns a sanitized version of environment variables
// where each value is truncated to first 4 characters followed by "..."
// This prevents sensitive information like API keys from being logged in full.
func RedactSecretMap(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	sanitized := make(map[string]string, len(env))
	for key, value := range env {
		sanitized[key] = RedactSecret(value)
	}
	return sanitized
}

// RedactURL returns a safe-to-log version of a URL by retaining only the scheme,
// host, and path. Userinfo (credentials), query parameters, and fragments are
// removed to prevent accidental leakage of secrets (e.g. api_key=..., token=...).
// If the input cannot be parsed as a URL, the literal string "<unparseable-url>" is
// returned instead so callers never log raw unverified input.
func RedactURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "<unparseable-url>"
	}
	safe := &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   u.Path,
	}
	return safe.String()
}

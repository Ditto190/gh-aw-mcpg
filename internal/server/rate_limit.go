package server

import (
	"time"

	"github.com/github/gh-aw-mcpg/internal/githubhttp"
	"github.com/github/gh-aw-mcpg/internal/mcpresult"
)

// extractRateLimitErrorText extracts the text content from a raw tool result
// that has been identified as a rate-limit error. Returns the original backend
// message so agents see the actual upstream error rather than a synthetic one.
func extractRateLimitErrorText(result interface{}) string {
	m, ok := result.(map[string]interface{})
	if !ok {
		logCircuitBreaker.Print("extractRateLimitErrorText: result is not a map, using default message")
		return "rate limit exceeded"
	}
	if text := mcpresult.ExtractTextContent(m); text != "" {
		return text
	}
	logCircuitBreaker.Print("extractRateLimitErrorText: no text content found, using default message")
	return "rate limit exceeded"
}

// isRateLimitToolResult reports whether a raw tool call result indicates
// a rate-limit error from the GitHub MCP server. It inspects the `isError`
// flag and the text content for well-known rate-limit phrases.
//
// The GitHub MCP server returns rate-limit errors as:
//
//	{"content":[{"type":"text","text":"... 403 API rate limit exceeded ..."}],"isError":true}
func isRateLimitToolResult(result interface{}) (bool, time.Time) {
	m, ok := result.(map[string]interface{})
	if !ok {
		return false, time.Time{}
	}

	// Only inspect error results.
	isErr, _ := m["isError"].(bool)
	if !isErr {
		return false, time.Time{}
	}

	text := mcpresult.ExtractTextContent(m)
	if githubhttp.IsRateLimitText(text) {
		resetAt := githubhttp.ParseRateLimitResetFromText(text)
		logCircuitBreaker.Printf("Rate limit detected in tool result: hasResetAt=%v", !resetAt.IsZero())
		return true, resetAt
	}
	return false, time.Time{}
}

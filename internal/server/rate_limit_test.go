package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsRateLimitText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "rate limit exceeded lowercase",
			text: "rate limit exceeded",
			want: true,
		},
		{
			name: "API rate limit exceeded",
			text: "API rate limit exceeded for user ID 12345",
			want: true,
		},
		{
			name: "rate limit with 403",
			text: "rate limit 403 Forbidden",
			want: true,
		},
		{
			name: "secondary rate limit",
			text: "You have exceeded a secondary rate limit",
			want: true,
		},
		{
			name: "too many requests",
			text: "too many requests, please slow down",
			want: true,
		},
		{
			name: "uppercase RATE LIMIT EXCEEDED",
			text: "RATE LIMIT EXCEEDED",
			want: true,
		},
		{
			name: "mixed case Rate Limit Exceeded",
			text: "Rate Limit Exceeded for this endpoint",
			want: true,
		},
		{
			name: "too many requests mixed case",
			text: "Too Many Requests",
			want: true,
		},
		{
			name: "normal error message",
			text: "repository not found",
			want: false,
		},
		{
			name: "empty string",
			text: "",
			want: false,
		},
		{
			name: "unrelated 403 error",
			text: "403 Forbidden: access denied",
			want: false,
		},
		{
			name: "partial match rate only",
			text: "rate of change is high",
			want: false,
		},
		{
			name: "limit only",
			text: "limit reached for this feature",
			want: false,
		},
		{
			name: "api rate limit without 403",
			text: "api rate limit reset in 60s",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isRateLimitText(tt.text)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractRateLimitErrorText(t *testing.T) {
	t.Parallel()

	makeTextContent := func(text string) map[string]interface{} {
		return map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": text,
				},
			},
			"isError": true,
		}
	}

	tests := []struct {
		name   string
		result interface{}
		want   string
	}{
		{
			name:   "extracts text from MCP error result",
			result: makeTextContent("API rate limit exceeded, rate reset in 60s"),
			want:   "API rate limit exceeded, rate reset in 60s",
		},
		{
			name:   "result is not a map returns default",
			result: "some string",
			want:   "rate limit exceeded",
		},
		{
			name:   "nil result returns default",
			result: nil,
			want:   "rate limit exceeded",
		},
		{
			name:   "map with no content returns default",
			result: map[string]interface{}{"isError": true},
			want:   "rate limit exceeded",
		},
		{
			name:   "map with empty text returns default",
			result: map[string]interface{}{"content": []interface{}{}, "isError": true},
			want:   "rate limit exceeded",
		},
		{
			name:   "integer result returns default",
			result: 42,
			want:   "rate limit exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractRateLimitErrorText(tt.result)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsRateLimitToolResult(t *testing.T) {
	t.Parallel()

	makeResult := func(text string, isError bool) map[string]interface{} {
		return map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": text,
				},
			},
			"isError": isError,
		}
	}

	tests := []struct {
		name       string
		result     interface{}
		wantIsRL   bool
		wantHasRst bool
	}{
		{
			name:       "rate limit error result",
			result:     makeResult("API rate limit exceeded for user", true),
			wantIsRL:   true,
			wantHasRst: false,
		},
		{
			name:       "rate limit with reset time",
			result:     makeResult("API rate limit exceeded [rate reset in 30s]", true),
			wantIsRL:   true,
			wantHasRst: true,
		},
		{
			name:       "success result not rate limit",
			result:     makeResult("list of repositories", false),
			wantIsRL:   false,
			wantHasRst: false,
		},
		{
			name:       "error result but not rate limit",
			result:     makeResult("repository not found", true),
			wantIsRL:   false,
			wantHasRst: false,
		},
		{
			name:       "not a map",
			result:     "some string",
			wantIsRL:   false,
			wantHasRst: false,
		},
		{
			name:       "nil",
			result:     nil,
			wantIsRL:   false,
			wantHasRst: false,
		},
		{
			name:       "too many requests error",
			result:     makeResult("too many requests from this client", true),
			wantIsRL:   true,
			wantHasRst: false,
		},
		{
			name:       "secondary rate limit",
			result:     makeResult("You have exceeded a secondary rate limit. Please wait a few minutes.", true),
			wantIsRL:   true,
			wantHasRst: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			isRL, resetAt := isRateLimitToolResult(tt.result)
			assert.Equal(t, tt.wantIsRL, isRL)
			if tt.wantHasRst {
				require.True(t, !resetAt.IsZero(), "expected non-zero resetAt time")
			} else {
				assert.True(t, resetAt.IsZero(), "expected zero resetAt time, got %v", resetAt)
			}
		})
	}
}

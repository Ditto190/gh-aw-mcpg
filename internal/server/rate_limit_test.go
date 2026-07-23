package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// extractRateLimitErrorText
// ---------------------------------------------------------------------------

func TestExtractRateLimitErrorTextBranches(t *testing.T) {
	t.Parallel()

	t.Run("non-map result returns default message", func(t *testing.T) {
		got := extractRateLimitErrorText("not a map")
		assert.Equal(t, "rate limit exceeded", got)
	})

	t.Run("nil result returns default message", func(t *testing.T) {
		got := extractRateLimitErrorText(nil)
		assert.Equal(t, "rate limit exceeded", got)
	})

	t.Run("integer result returns default message", func(t *testing.T) {
		got := extractRateLimitErrorText(42)
		assert.Equal(t, "rate limit exceeded", got)
	})

	t.Run("map without content returns default message", func(t *testing.T) {
		got := extractRateLimitErrorText(map[string]interface{}{
			"isError": true,
		})
		assert.Equal(t, "rate limit exceeded", got)
	})

	t.Run("map with empty content slice returns default message", func(t *testing.T) {
		got := extractRateLimitErrorText(map[string]interface{}{
			"isError": true,
			"content": []interface{}{},
		})
		assert.Equal(t, "rate limit exceeded", got)
	})

	t.Run("map with text content returns the text", func(t *testing.T) {
		got := extractRateLimitErrorText(map[string]interface{}{
			"isError": true,
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "API rate limit exceeded for user ID 12345",
				},
			},
		})
		assert.Equal(t, "API rate limit exceeded for user ID 12345", got)
	})

	t.Run("map with non-text content type returns default message", func(t *testing.T) {
		got := extractRateLimitErrorText(map[string]interface{}{
			"isError": true,
			"content": []interface{}{
				map[string]interface{}{
					"type": "image",
					"data": "base64data",
				},
			},
		})
		assert.Equal(t, "rate limit exceeded", got)
	})

	t.Run("map with multiple content items returns first text", func(t *testing.T) {
		got := extractRateLimitErrorText(map[string]interface{}{
			"isError": true,
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "secondary rate limit triggered",
				},
				map[string]interface{}{
					"type": "text",
					"text": "additional info",
				},
			},
		})
		assert.Equal(t, "secondary rate limit triggeredadditional info", got)
	})
}

// ---------------------------------------------------------------------------
// isRateLimitToolResult
// ---------------------------------------------------------------------------

func TestIsRateLimitToolResultBranches(t *testing.T) {
	t.Parallel()

	t.Run("non-map result returns false", func(t *testing.T) {
		ok, resetAt := isRateLimitToolResult("not a map")
		assert.False(t, ok)
		assert.True(t, resetAt.IsZero())
	})

	t.Run("nil result returns false", func(t *testing.T) {
		ok, resetAt := isRateLimitToolResult(nil)
		assert.False(t, ok)
		assert.True(t, resetAt.IsZero())
	})

	t.Run("map with isError false returns false even with rate limit text", func(t *testing.T) {
		result := map[string]interface{}{
			"isError": false,
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "rate limit exceeded",
				},
			},
		}
		ok, resetAt := isRateLimitToolResult(result)
		assert.False(t, ok)
		assert.True(t, resetAt.IsZero())
	})

	t.Run("map without isError field returns false", func(t *testing.T) {
		result := map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "rate limit exceeded",
				},
			},
		}
		ok, resetAt := isRateLimitToolResult(result)
		assert.False(t, ok)
		assert.True(t, resetAt.IsZero())
	})

	t.Run("isError true with non-rate-limit text returns false", func(t *testing.T) {
		result := map[string]interface{}{
			"isError": true,
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "repository not found",
				},
			},
		}
		ok, resetAt := isRateLimitToolResult(result)
		assert.False(t, ok)
		assert.True(t, resetAt.IsZero())
	})

	t.Run("isError true with rate limit exceeded text returns true", func(t *testing.T) {
		result := map[string]interface{}{
			"isError": true,
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "API rate limit exceeded for user ID 12345",
				},
			},
		}
		ok, resetAt := isRateLimitToolResult(result)
		assert.True(t, ok)
		assert.True(t, resetAt.IsZero(), "no reset time in text so should be zero")
	})

	t.Run("isError true with secondary rate limit text returns true", func(t *testing.T) {
		result := map[string]interface{}{
			"isError": true,
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "You have exceeded a secondary rate limit and have been temporarily blocked",
				},
			},
		}
		ok, resetAt := isRateLimitToolResult(result)
		assert.True(t, ok)
		assert.True(t, resetAt.IsZero())
	})

	t.Run("isError true with too many requests text returns true", func(t *testing.T) {
		result := map[string]interface{}{
			"isError": true,
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Too Many Requests: please slow down",
				},
			},
		}
		ok, resetAt := isRateLimitToolResult(result)
		assert.True(t, ok)
		assert.True(t, resetAt.IsZero())
	})

	t.Run("isError true with rate limit 403 returns true", func(t *testing.T) {
		result := map[string]interface{}{
			"isError": true,
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "rate limit 403 Forbidden",
				},
			},
		}
		ok, resetAt := isRateLimitToolResult(result)
		assert.True(t, ok)
		assert.True(t, resetAt.IsZero())
	})

	t.Run("isError true with rate limit text and reset time returns non-zero time", func(t *testing.T) {
		result := map[string]interface{}{
			"isError": true,
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "API rate limit exceeded. rate reset in 60s. Please wait.",
				},
			},
		}
		before := time.Now()
		ok, resetAt := isRateLimitToolResult(result)
		after := time.Now().Add(2 * time.Minute)

		require.True(t, ok)
		assert.False(t, resetAt.IsZero(), "reset time should be non-zero when text contains reset info")
		assert.True(t, resetAt.After(before), "reset time should be in the future")
		assert.True(t, resetAt.Before(after), "reset time should be within 2 minutes")
	})

	t.Run("isError true with empty content returns false", func(t *testing.T) {
		result := map[string]interface{}{
			"isError": true,
			"content": []interface{}{},
		}
		ok, resetAt := isRateLimitToolResult(result)
		assert.False(t, ok)
		assert.True(t, resetAt.IsZero())
	})

	t.Run("isError true with no content key returns false", func(t *testing.T) {
		result := map[string]interface{}{
			"isError": true,
		}
		ok, resetAt := isRateLimitToolResult(result)
		assert.False(t, ok)
		assert.True(t, resetAt.IsZero())
	})
}

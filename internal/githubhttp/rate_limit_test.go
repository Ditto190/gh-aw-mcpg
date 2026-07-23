package githubhttp

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
			assert.Equal(t, tt.want, IsRateLimitText(tt.text))
		})
	}
}

// TestParseRateLimitResetFromText verifies all branches of the text-based
// rate-limit reset parser.
func TestParseRateLimitResetFromText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		text      string
		wantZero  bool
		minOffset time.Duration // minimum expected offset from now (for non-zero results)
		maxOffset time.Duration // maximum expected offset from now (for non-zero results)
	}{
		{
			name:     "empty string",
			text:     "",
			wantZero: true,
		},
		{
			name:     "unrelated text",
			text:     "API rate limit exceeded for user.",
			wantZero: true,
		},
		{
			name:     "pattern absent",
			text:     "some other error message with no rate info",
			wantZero: true,
		},
		{
			name:     "pattern present but no terminator",
			text:     "API rate limit exceeded [rate reset in 42",
			wantZero: true,
		},
		{
			name:      "seconds terminator",
			text:      "API rate limit exceeded [rate reset in 60s]",
			wantZero:  false,
			minOffset: 59 * time.Second,
			maxOffset: 61 * time.Second,
		},
		{
			name:      "bracket terminator",
			text:      "API rate limit exceeded [rate reset in 30]",
			wantZero:  false,
			minOffset: 29 * time.Second,
			maxOffset: 31 * time.Second,
		},
		{
			name:      "paren terminator",
			text:      "API rate limit exceeded (rate reset in 45)",
			wantZero:  false,
			minOffset: 44 * time.Second,
			maxOffset: 46 * time.Second,
		},
		{
			name:     "zero seconds",
			text:     "rate reset in 0s",
			wantZero: true,
		},
		{
			name:     "negative seconds",
			text:     "rate reset in -5s",
			wantZero: true,
		},
		{
			name:     "non-numeric value",
			text:     "rate reset in abcs",
			wantZero: true,
		},
		{
			name:      "case insensitive uppercase",
			text:      "API Rate Limit Exceeded [RATE RESET IN 10s]",
			wantZero:  false,
			minOffset: 9 * time.Second,
			maxOffset: 11 * time.Second,
		},
		{
			name:      "mixed case",
			text:      "rate Reset In 20s",
			wantZero:  false,
			minOffset: 19 * time.Second,
			maxOffset: 21 * time.Second,
		},
		{
			name:      "large value",
			text:      "rate reset in 3600s",
			wantZero:  false,
			minOffset: 3599 * time.Second,
			maxOffset: 3601 * time.Second,
		},
		{
			name:      "whitespace before seconds",
			text:      "rate reset in  42s",
			wantZero:  false,
			minOffset: 41 * time.Second,
			maxOffset: 43 * time.Second,
		},
		{
			name:      "long text with pattern embedded",
			text:      strings.Repeat("x", 200) + " rate reset in 5s " + strings.Repeat("y", 200),
			wantZero:  false,
			minOffset: 4 * time.Second,
			maxOffset: 6 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			before := time.Now()
			got := ParseRateLimitResetFromText(tt.text)
			after := time.Now()

			if tt.wantZero {
				assert.True(t, got.IsZero(), "expected zero time, got %v", got)
				return
			}

			assert.False(t, got.IsZero(), "expected non-zero time")
			// The returned time should be in the range [before+minOffset, after+maxOffset].
			assert.True(t, !got.Before(before.Add(tt.minOffset)),
				"reset time %v is too early (expected at least %v after %v)", got, tt.minOffset, before)
			assert.True(t, !got.After(after.Add(tt.maxOffset)),
				"reset time %v is too late (expected at most %v after %v)", got, tt.maxOffset, after)
		})
	}
}

func TestRateLimitSignal(t *testing.T) {
	t.Parallel()

	t.Run("nil response is not rate limited", func(t *testing.T) {
		t.Parallel()
		limited, reset, remaining := RateLimitSignal(nil)
		assert.False(t, limited)
		assert.Empty(t, reset)
		assert.Empty(t, remaining)
	})

	t.Run("429 status is rate limited", func(t *testing.T) {
		t.Parallel()
		resp := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"X-Ratelimit-Reset": []string{"12345"}},
		}
		limited, reset, remaining := RateLimitSignal(resp)
		assert.True(t, limited)
		assert.Equal(t, "12345", reset)
		assert.Equal(t, "", remaining)
	})

	t.Run("remaining zero is rate limited", func(t *testing.T) {
		t.Parallel()
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"X-Ratelimit-Remaining": []string{"0"}},
		}
		limited, _, remaining := RateLimitSignal(resp)
		assert.True(t, limited)
		assert.Equal(t, "0", remaining)
	})

	t.Run("non-rate-limited response", func(t *testing.T) {
		t.Parallel()
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"X-Ratelimit-Remaining": []string{"100"}},
		}
		limited, _, remaining := RateLimitSignal(resp)
		assert.False(t, limited)
		assert.Equal(t, "100", remaining)
	})
}

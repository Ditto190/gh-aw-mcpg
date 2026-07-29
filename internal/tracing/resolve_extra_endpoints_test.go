package tracing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveCommaSeparatedExtraEndpoints verifies direct behavior of the private
// resolveCommaSeparatedExtraEndpoints function, covering normal paths, whitespace
// trimming, empty-entry filtering, and multi-endpoint parsing.
func TestResolveCommaSeparatedExtraEndpoints(t *testing.T) {
	const signalPath = "/v1/traces"

	tests := []struct {
		name      string
		raw       string
		wantLen   int
		wantURLs  []string
		wantEmpty bool
	}{
		{
			name:     "single valid endpoint",
			raw:      "https://collector.example.com",
			wantLen:  1,
			wantURLs: []string{"https://collector.example.com/v1/traces"},
		},
		{
			name:     "multiple valid endpoints",
			raw:      "https://a.example.com,https://b.example.com",
			wantLen:  2,
			wantURLs: []string{"https://a.example.com/v1/traces", "https://b.example.com/v1/traces"},
		},
		{
			name:     "endpoint with trailing slash is normalized",
			raw:      "https://collector.example.com/",
			wantLen:  1,
			wantURLs: []string{"https://collector.example.com/v1/traces"},
		},
		{
			name:     "endpoint already has signal path",
			raw:      "https://collector.example.com/v1/traces",
			wantLen:  1,
			wantURLs: []string{"https://collector.example.com/v1/traces"},
		},
		{
			name:      "empty string",
			raw:       "",
			wantEmpty: true,
		},
		{
			name:      "only commas",
			raw:       ",,,",
			wantEmpty: true,
		},
		{
			name:      "only whitespace entries",
			raw:       "  ,  ,  ",
			wantEmpty: true,
		},
		{
			name:     "skips empty entries, keeps valid",
			raw:      ",https://a.example.com,,https://b.example.com,",
			wantLen:  2,
			wantURLs: []string{"https://a.example.com/v1/traces", "https://b.example.com/v1/traces"},
		},
		{
			name:     "whitespace around endpoint is trimmed",
			raw:      "  https://collector.example.com  ",
			wantLen:  1,
			wantURLs: []string{"https://collector.example.com/v1/traces"},
		},
		{
			name:     "custom signal path appended",
			raw:      "https://collector.example.com",
			wantLen:  1,
			wantURLs: []string{"https://collector.example.com/custom/path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sp := signalPath
			if tt.name == "custom signal path appended" {
				sp = "/custom/path"
			}
			got := resolveCommaSeparatedExtraEndpoints(tt.raw, sp)

			if tt.wantEmpty {
				assert.Empty(t, got)
				return
			}

			require.Len(t, got, tt.wantLen)
			for i, wantURL := range tt.wantURLs {
				assert.Equal(t, wantURL, got[i].URL, "endpoint[%d].URL mismatch", i)
				assert.Nil(t, got[i].Headers, "endpoint[%d] should have no headers", i)
			}
		})
	}
}

// TestResolveJSONExtraEndpoints verifies direct behavior of resolveJSONExtraEndpoints,
// covering valid JSON arrays, per-endpoint headers, empty-URL filtering, and invalid JSON.
func TestResolveJSONExtraEndpoints(t *testing.T) {
	const signalPath = "/v1/traces"

	t.Run("single endpoint without headers", func(t *testing.T) {
		raw := `[{"url":"https://collector.example.com"}]`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		require.Len(t, got, 1)
		assert.Equal(t, "https://collector.example.com/v1/traces", got[0].URL)
		assert.Nil(t, got[0].Headers)
	})

	t.Run("single endpoint with headers", func(t *testing.T) {
		raw := `[{"url":"https://collector.example.com","headers":"Authorization=Bearer token123"}]`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		require.Len(t, got, 1)
		assert.Equal(t, "https://collector.example.com/v1/traces", got[0].URL)
		assert.Equal(t, map[string]string{"Authorization": "Bearer token123"}, got[0].Headers)
	})

	t.Run("multiple endpoints with mixed headers", func(t *testing.T) {
		raw := `[{"url":"https://a.example.com","headers":"X-Key=val1"},{"url":"https://b.example.com"}]`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		require.Len(t, got, 2)
		assert.Equal(t, "https://a.example.com/v1/traces", got[0].URL)
		assert.Equal(t, map[string]string{"X-Key": "val1"}, got[0].Headers)
		assert.Equal(t, "https://b.example.com/v1/traces", got[1].URL)
		assert.Nil(t, got[1].Headers)
	})

	t.Run("empty URL entries are filtered out", func(t *testing.T) {
		raw := `[{"url":""},{"url":"https://b.example.com"},{"url":"   "}]`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		require.Len(t, got, 1)
		assert.Equal(t, "https://b.example.com/v1/traces", got[0].URL)
	})

	t.Run("all empty URLs returns empty slice", func(t *testing.T) {
		raw := `[{"url":""},{"url":"  "}]`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		assert.Empty(t, got)
	})

	t.Run("empty JSON array returns empty slice", func(t *testing.T) {
		raw := `[]`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		assert.Empty(t, got)
	})

	t.Run("invalid JSON returns nil", func(t *testing.T) {
		raw := `not-json`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		assert.Nil(t, got)
	})

	t.Run("malformed JSON object instead of array returns nil", func(t *testing.T) {
		raw := `{"url":"https://collector.example.com"}`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		assert.Nil(t, got)
	})

	t.Run("endpoint already has signal path", func(t *testing.T) {
		raw := `[{"url":"https://collector.example.com/v1/traces"}]`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		require.Len(t, got, 1)
		assert.Equal(t, "https://collector.example.com/v1/traces", got[0].URL)
	})

	t.Run("empty headers string results in no headers map", func(t *testing.T) {
		raw := `[{"url":"https://collector.example.com","headers":""}]`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		require.Len(t, got, 1)
		assert.Equal(t, "https://collector.example.com/v1/traces", got[0].URL)
		assert.Nil(t, got[0].Headers)
	})

	t.Run("malformed headers result in no headers map", func(t *testing.T) {
		raw := `[{"url":"https://collector.example.com","headers":"malformed"}]`
		got := resolveJSONExtraEndpoints(raw, signalPath)
		require.Len(t, got, 1)
		assert.Equal(t, "https://collector.example.com/v1/traces", got[0].URL)
		assert.Nil(t, got[0].Headers)
	})

	t.Run("custom signal path is used", func(t *testing.T) {
		raw := `[{"url":"https://collector.example.com"}]`
		got := resolveJSONExtraEndpoints(raw, "/custom/signal")
		require.Len(t, got, 1)
		assert.Equal(t, "https://collector.example.com/custom/signal", got[0].URL)
	})
}

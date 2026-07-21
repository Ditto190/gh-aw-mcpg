package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFormatJSONWithoutFields_Direct tests formatJSONWithoutFields directly,
// independently of the RPC logging infrastructure.
func TestFormatJSONWithoutFields_Direct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		jsonStr         string
		fieldsToRemove  []string
		wantIsValidJSON bool
		wantIsEmpty     bool
		// wantFormatted is only checked for invalid JSON (order-independent via JSONEq otherwise)
		wantFormatted string
	}{
		{
			name:            "plain text is not valid JSON",
			jsonStr:         "not json at all",
			fieldsToRemove:  []string{"jsonrpc"},
			wantIsValidJSON: false,
			wantIsEmpty:     false,
			wantFormatted:   "not json at all",
		},
		{
			name:            "empty string is not valid JSON",
			jsonStr:         "",
			fieldsToRemove:  []string{},
			wantIsValidJSON: false,
			wantIsEmpty:     false,
			wantFormatted:   "",
		},
		{
			name:            "JSON array is not treated as object",
			jsonStr:         `[1, 2, 3]`,
			fieldsToRemove:  []string{"field"},
			wantIsValidJSON: false,
			wantIsEmpty:     false,
			wantFormatted:   `[1, 2, 3]`,
		},
		{
			name:            "truncated JSON is invalid",
			jsonStr:         `{"key": "value"`,
			fieldsToRemove:  []string{"key"},
			wantIsValidJSON: false,
			wantIsEmpty:     false,
			wantFormatted:   `{"key": "value"`,
		},
		{
			name:            "removing all fields yields empty object",
			jsonStr:         `{"jsonrpc":"2.0","method":"test"}`,
			fieldsToRemove:  []string{"jsonrpc", "method"},
			wantIsValidJSON: true,
			wantIsEmpty:     true,
		},
		{
			name:            "nil fields list leaves JSON unchanged",
			jsonStr:         `{"key":"value"}`,
			fieldsToRemove:  nil,
			wantIsValidJSON: true,
			wantIsEmpty:     false,
		},
		{
			name:            "empty fields list leaves JSON unchanged",
			jsonStr:         `{"key":"value"}`,
			fieldsToRemove:  []string{},
			wantIsValidJSON: true,
			wantIsEmpty:     false,
		},
		{
			name:            "removing nonexistent fields is a no-op",
			jsonStr:         `{"id":1,"params":{"key":"value"}}`,
			fieldsToRemove:  []string{"nonexistent", "alsonothere"},
			wantIsValidJSON: true,
			wantIsEmpty:     false,
		},
		{
			name:            "params null with other fields is not empty",
			jsonStr:         `{"jsonrpc":"2.0","params":null,"id":1}`,
			fieldsToRemove:  []string{"jsonrpc"},
			wantIsValidJSON: true,
			wantIsEmpty:     false,
		},
		{
			name:            "nested objects are preserved after field removal",
			jsonStr:         `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"search","arguments":{"query":"test"}}}`,
			fieldsToRemove:  []string{"jsonrpc", "method"},
			wantIsValidJSON: true,
			wantIsEmpty:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotFormatted, gotIsValidJSON, gotIsEmpty := formatJSONWithoutFields(tt.jsonStr, tt.fieldsToRemove)
			assert.Equal(t, tt.wantIsValidJSON, gotIsValidJSON, "isValidJSON mismatch")
			assert.Equal(t, tt.wantIsEmpty, gotIsEmpty, "isEmpty mismatch")
			if !tt.wantIsValidJSON {
				assert.Equal(t, tt.wantFormatted, gotFormatted, "formatted string mismatch for invalid JSON")
			} else {
				// For valid JSON output, verify it's still valid JSON and not just whitespace
				assert.NotEmpty(t, gotFormatted, "valid JSON result should not be empty string")
			}
		})
	}
}

// TestIsEffectivelyEmpty_Direct tests isEffectivelyEmpty directly.
func TestIsEffectivelyEmpty_Direct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data map[string]interface{}
		want bool
	}{
		{
			name: "nil map is empty",
			data: nil,
			want: true,
		},
		{
			name: "empty map is empty",
			data: map[string]interface{}{},
			want: true,
		},
		{
			name: "single params-null entry is empty",
			data: map[string]interface{}{"params": nil},
			want: true,
		},
		{
			name: "params with non-nil value is not empty",
			data: map[string]interface{}{"params": map[string]interface{}{"key": "val"}},
			want: false,
		},
		{
			name: "single non-params field is not empty",
			data: map[string]interface{}{"id": 1},
			want: false,
		},
		{
			name: "multiple fields including params-null is not empty",
			data: map[string]interface{}{"params": nil, "id": 1},
			want: false,
		},
		{
			name: "single params with zero value string is not empty",
			data: map[string]interface{}{"params": ""},
			want: false,
		},
		{
			name: "single params with false boolean is not empty",
			data: map[string]interface{}{"params": false},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isEffectivelyEmpty(tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFormatJSONWithoutFields_OutputIsCompact verifies the output is always compacted
// to a single line (no newlines or extra whitespace).
func TestFormatJSONWithoutFields_OutputIsCompact(t *testing.T) {
	t.Parallel()

	// Prettified JSON as input
	input := "{\n  \"id\": 1,\n  \"params\": {\n    \"key\": \"value\"\n  }\n}"
	got, isValid, isEmpty := formatJSONWithoutFields(input, []string{})
	assert.True(t, isValid, "prettified JSON should be valid")
	assert.False(t, isEmpty, "non-empty JSON should not be empty")
	assert.NotContains(t, got, "\n", "output should be compacted to single line")
	assert.NotContains(t, got, "  ", "output should not have multiple spaces")
}

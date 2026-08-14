package middleware

// Tests targeting previously uncovered branches in jqschema.go:
//   - compileToolResponseFilterInternal: cache poisoned with a non-*gojq.Code
//     value (defensive branch that should never happen in practice)
//   - tryApplyToolResponseFilter: json.Marshal(filteredPayload) failure for the
//     text-content path (filter produces a non-JSON-serializable value such as
//     +Inf)
//   - wrapToolHandler: json.Unmarshal failure in the schema-generation "default"
//     branch when data does not match any of the fast native types
//   - wrapToolHandler: successful schema generation contract surrounding the
//     json.Marshal(rewrittenResponse) fallback branch

import (
	"context"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// compileToolResponseFilterInternal: poisoned cache entry (lines 292-295)
// ---------------------------------------------------------------------------

// TestCompileToolResponseFilter_PoisonedCacheEntry verifies the defensive
// "should never happen" branch that fires when filterCodeCache holds a value
// that is not a *gojq.Code for the given key. This can only occur if another
// part of the code (or a test) stores an unexpected type under the same key,
// but the function must still fail safely with a descriptive error rather
// than panicking on the type assertion.
func TestCompileToolResponseFilter_PoisonedCacheEntry(t *testing.T) {
	// Use a filter string that is exceedingly unlikely to collide with any
	// other test's cache key.
	const filter = "__poisoned_cache_key_test__"

	// Poison the shared cache with a value of the wrong type.
	filterCodeCache.Store(filter, "not-a-gojq-code")
	t.Cleanup(func() { filterCodeCache.Delete(filter) })

	code, err := CompileToolResponseFilter(filter)
	require.Error(t, err, "poisoned cache entry should produce an error rather than panicking")
	assert.Nil(t, code)
	assert.ErrorContains(t, err, "internal error: unexpected cached value type for filter")
}

// ---------------------------------------------------------------------------
// tryApplyToolResponseFilter: filtered text payload marshal failure (lines 376-380)
// ---------------------------------------------------------------------------

// TestWrapToolHandlerWithFilter_TextPayloadMarshalFailure verifies that when
// the jq filter applied to a text-content JSON payload produces a value that
// cannot be marshaled back to JSON (such as +Inf, produced by the built-in
// "infinite" jq function), tryApplyToolResponseFilter logs the failure and
// falls back to the original response instead of propagating an error.
func TestWrapToolHandlerWithFilter_TextPayloadMarshalFailure(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()

	originalResult := &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: `{"value":1}`}},
	}
	originalData := map[string]any{"value": float64(1)}

	mockHandler := func(_ context.Context, _ *sdk.CallToolRequest, _ any) (*sdk.CallToolResult, any, error) {
		return originalResult, originalData, nil
	}

	// "infinite" always evaluates to +Inf regardless of input, which
	// json.Marshal cannot serialize ("json: unsupported value: +Inf").
	wrapped := WrapToolHandlerWithFilter(mockHandler, "test_tool", baseDir, "", 0, testGetSessionID, "infinite")
	result, data, err := wrapped(context.Background(), &sdk.CallToolRequest{}, nil)

	require.NoError(t, err, "marshal failure inside the filter path should not propagate as a handler error")
	require.NotNil(t, result)

	// The fallback returns the original result/data pair to the caller (i.e.
	// the pre-filter payload, not a filtered +Inf value). Because
	// sizeThreshold is 0, the unfiltered payload is then routed through the
	// oversized-payload path and wrapped into PayloadMetadata; either way,
	// the original data (not the failed filter output) must be what
	// downstream processing observes.
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(*sdk.TextContent)
	require.True(t, ok)
	assert.NotNil(t, data)

	if meta, isMeta := data.(PayloadMetadata); isMeta {
		assert.Equal(t, `{"value":1}`, meta.PayloadPreview, "preview should reflect the original unfiltered payload")
	} else {
		assert.Equal(t, `{"value":1}`, tc.Text, "original text content should be preserved when filtered payload cannot be marshaled")
	}
}

// ---------------------------------------------------------------------------
// wrapToolHandler: json.Unmarshal failure in schema default branch (lines 621-623)
// ---------------------------------------------------------------------------

// invalidJSONMarshaler marshals to syntactically invalid JSON. It is used to
// force the default branch of the schema-generation type switch (which
// re-unmarshals payloadJSON) down the json.Unmarshal error path, since the
// value's static type does not match any of the fast native cases handled
// directly (map[string]any, []any, string, float64, bool, nil, json.Number).
type invalidJSONMarshaler struct{}

func (invalidJSONMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`{not valid json`), nil
}

func TestWrapToolHandler_SchemaDefaultBranch_UnmarshalFailure(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()

	originalData := invalidJSONMarshaler{}
	mockHandler := func(_ context.Context, _ *sdk.CallToolRequest, _ any) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: "result"}},
		}, originalData, nil
	}

	// sizeThreshold = 0 forces the file-storage + schema-generation path even
	// for a tiny payload.
	wrapped := WrapToolHandler(mockHandler, "test_tool", baseDir, "", 0, testGetSessionID)
	result, data, err := wrapped(context.Background(), &sdk.CallToolRequest{}, nil)

	require.NoError(t, err, "schema generation failure should not propagate as a handler error")
	require.NotNil(t, result)

	// Because json.Unmarshal(payloadJSON, &jsonData) fails, applyJqSchema is
	// never reached and the schemaErr branch falls back to the original
	// result/data instead of a PayloadMetadata struct.
	_, isMetadata := data.(PayloadMetadata)
	assert.False(t, isMetadata, "invalid JSON payload should cause schema generation to fail, returning original data")
	assert.Equal(t, originalData, data)
}

// ---------------------------------------------------------------------------
// wrapToolHandler: successful schema generation contract (regression guard for
// the code paths surrounding the json.Marshal(rewrittenResponse) branch at
// lines 680-685; the walk_schema jq filter used internally always produces
// JSON-serializable string schema values, so the marshal-failure branch
// itself cannot be triggered through the public API without an internal
// hook, but the surrounding fallback contract is still verified here).
// ---------------------------------------------------------------------------

// TestWrapToolHandler_SchemaGenerationSuccess_ReturnsPayloadMetadata verifies
// that a normal oversized payload with successful schema generation produces
// a well-formed PayloadMetadata response (the success path that immediately
// precedes the marshal-failure fallback branch).
func TestWrapToolHandler_SchemaGenerationSuccess_ReturnsPayloadMetadata(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()

	originalResult := &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: "result"}},
	}
	originalData := map[string]any{"key": "value"}

	mockHandler := func(_ context.Context, _ *sdk.CallToolRequest, _ any) (*sdk.CallToolResult, any, error) {
		return originalResult, originalData, nil
	}

	wrapped := WrapToolHandler(mockHandler, "test_tool", baseDir, "", 0, testGetSessionID)
	result, data, err := wrapped(context.Background(), &sdk.CallToolRequest{}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	meta, ok := data.(PayloadMetadata)
	require.True(t, ok, "expected PayloadMetadata for oversized payload with successful schema generation")
	assert.NotEmpty(t, meta.PayloadPath)
	assert.NotNil(t, meta.PayloadSchema)
}

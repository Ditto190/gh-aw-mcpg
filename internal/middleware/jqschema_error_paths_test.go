package middleware

// Tests for previously uncovered error branches in jqschema.go:
//   - CompileToolResponseFilter compile error (parse ok, compile fails: undeclared variable)
//   - wrapToolHandler data marshal error (data is an unmarshalable type like chan int)
//   - wrapToolHandler json.Number.Float64() overflow path (json.Number("1e309"))

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// CompileToolResponseFilter: compile error (parse succeeds, compile fails)
// ---------------------------------------------------------------------------

// TestCompileToolResponseFilter_CompileError verifies that an expression that
// parses successfully but fails to compile (e.g., references an undeclared
// variable) returns an error containing "failed to compile tool response filter".
//
// This exercises the branch at lines 275-277 of jqschema.go where gojq.Compile
// returns an error without a preceding gojq.Parse failure.
func TestCompileToolResponseFilter_CompileError(t *testing.T) {
	t.Parallel()

	// "$x" parses without error but fails to compile because $x is not declared
	// as a variable (WithVariables is not in secureCompileOpts).
	code, err := CompileToolResponseFilter("$x")
	require.Error(t, err, "should fail when filter references undeclared variable")
	assert.Nil(t, code, "code should be nil on compile error")
	assert.ErrorContains(t, err, "failed to compile tool response filter")
}

// TestCompileToolResponseFilterWithVars_CompileError verifies the analogous
// compile-error branch in CompileToolResponseFilterWithVars (lines 320-323).
// A filter that references "$undeclared" when only "$declared" is named fails.
func TestCompileToolResponseFilterWithVars_CompileError(t *testing.T) {
	t.Parallel()

	// "$undeclared" is referenced in the filter but only "$declared" is listed
	// in varNames, so gojq.Compile returns "variable not defined".
	code, err := CompileToolResponseFilterWithVars(". + $undeclared", []string{"$declared"})
	require.Error(t, err, "should fail when filter references undeclared variable")
	assert.Nil(t, code, "code should be nil on compile error")
	assert.ErrorContains(t, err, "failed to compile tool response filter")
}

// ---------------------------------------------------------------------------
// wrapToolHandler: json.Marshal(data) failure (lines 581-584)
// ---------------------------------------------------------------------------

// TestWrapToolHandler_DataMarshalFailure verifies that when json.Marshal(data)
// fails (because data is an unmarshalable type like a channel), wrapToolHandler
// returns the original result and data without error rather than propagating
// the marshal failure.
//
// This exercises lines 581-584 of jqschema.go.
func TestWrapToolHandler_DataMarshalFailure(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()

	// A channel is not JSON-serializable; json.Marshal will return an error.
	ch := make(chan int)
	originalResult := &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: "ok"}},
	}

	mockHandler := func(_ context.Context, _ *sdk.CallToolRequest, _ any) (*sdk.CallToolResult, any, error) {
		return originalResult, ch, nil
	}

	// sizeThreshold = 0 forces the marshal path (never takes the fast path).
	wrapped := WrapToolHandler(mockHandler, "test_tool", baseDir, "", 0, testGetSessionID)
	result, data, err := wrapped(context.Background(), &sdk.CallToolRequest{}, nil)

	require.NoError(t, err, "data marshal failure should not propagate as an error")
	// Pointer identity confirms the original result was returned without copying or mutating.
	assert.Same(t, originalResult, result, "original result pointer should be returned on marshal failure")
	require.Len(t, result.Content, 1)
	assert.Equal(t, "ok", result.Content[0].(*sdk.TextContent).Text, "result content should be unchanged on marshal failure")
	assert.Equal(t, ch, data, "original data (channel) should be returned on marshal failure")
}

// ---------------------------------------------------------------------------
// wrapToolHandler: json.Number.Float64() overflow (lines 630-631)
// ---------------------------------------------------------------------------

// TestWrapToolHandler_JsonNumberFloat64Overflow verifies that when wrapToolHandler
// encounters a json.Number value for data that cannot be converted to float64
// (e.g. "1e309" which overflows), the schema-generation step returns an error
// and the handler falls back to returning the original result and data.
//
// This exercises lines 629-631 of jqschema.go (json.Number.Float64() error path).
func TestWrapToolHandler_JsonNumberFloat64Overflow(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()

	// json.Number("1e309") marshals to the string "1e309" in JSON, so
	// json.Marshal(data) succeeds and returns a 5-byte payload. Use a
	// sizeThreshold of 0 to force the large-payload path.
	//
	// However, json.Number("1e309").Float64() returns an error
	// (strconv.ErrRange), which triggers lines 630-631 and causes
	// the schema step to fail. The handler returns the original result.
	overflow := json.Number("1e309")

	originalResult := &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: "ok"}},
	}
	mockHandler := func(_ context.Context, _ *sdk.CallToolRequest, _ any) (*sdk.CallToolResult, any, error) {
		return originalResult, overflow, nil
	}

	wrapped := WrapToolHandler(mockHandler, "test_tool", baseDir, "", 0, testGetSessionID)
	result, data, err := wrapped(context.Background(), &sdk.CallToolRequest{}, nil)

	require.NoError(t, err, "json.Number overflow should not propagate as handler error")
	// The original result pointer must be returned when schema generation fails.
	require.NotNil(t, result)
	assert.Same(t, originalResult, result, "original result should be returned when json.Number conversion fails")

	// When schema generation fails (lines 647-650), the original result and data
	// are returned (not a PayloadMetadata struct).
	_, isMetadata := data.(PayloadMetadata)
	assert.False(t, isMetadata, "overflow should cause schema error, returning original data not PayloadMetadata")
	assert.Equal(t, overflow, data, "original data should be returned when json.Number conversion fails")
}

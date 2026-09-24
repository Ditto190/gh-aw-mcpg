package jqutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecureCompileOpts_DisablesENV(t *testing.T) {
	for _, filter := range []string{"$ENV", "env"} {
		t.Run(filter, func(t *testing.T) {
			query, err := gojq.Parse(filter)
			require.NoError(t, err)

			code, err := gojq.Compile(query, SecureCompileOpts...)
			require.NoError(t, err)

			iter := code.RunWithContext(context.Background(), nil)
			v, ok := iter.Next()
			require.True(t, ok, "expected a result from %s query", filter)

			envMap, ok := v.(map[string]any)
			require.True(t, ok, "expected %s to return a map, got %T", filter, v)
			assert.Empty(t, envMap, "%s should be empty when environment loader is disabled", filter)
		})
	}
}

func TestSecureCompileOpts_AllowsNormalFilters(t *testing.T) {
	query, err := gojq.Parse(`.name`)
	require.NoError(t, err)

	code, err := gojq.Compile(query, SecureCompileOpts...)
	require.NoError(t, err)

	input := map[string]any{"name": "test-value", "count": 42}
	iter := code.RunWithContext(context.Background(), input)
	v, ok := iter.Next()
	require.True(t, ok)
	assert.Equal(t, "test-value", v)
}

func TestCompileOptsWithVariables(t *testing.T) {
	varNames := []string{"$x", "$y"}
	opts := CompileOptsWithVariables(varNames)

	// Should have SecureCompileOpts + 1 (WithVariables)
	assert.Len(t, opts, len(SecureCompileOpts)+1)

	// Verify the options work: compile a filter referencing the variables
	query, err := gojq.Parse(`{a: $x, b: $y}`)
	require.NoError(t, err)

	code, err := gojq.Compile(query, opts...)
	require.NoError(t, err)

	iter := code.RunWithContext(context.Background(), nil, "hello", 42)
	v, ok := iter.Next()
	require.True(t, ok)

	result, ok := v.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hello", result["a"])
	assert.Equal(t, 42, result["b"])
}

func TestCompileOptsWithVariables_DoesNotMutateSharedSlice(t *testing.T) {
	origLen := len(SecureCompileOpts)
	origCap := cap(SecureCompileOpts)

	_ = CompileOptsWithVariables([]string{"$a"})
	_ = CompileOptsWithVariables([]string{"$b", "$c"})

	assert.Len(t, SecureCompileOpts, origLen, "SecureCompileOpts length should not change")
	assert.Equal(t, origCap, cap(SecureCompileOpts), "SecureCompileOpts capacity should not change")
}

func TestParseErrorDetails_WithParseError(t *testing.T) {
	_, err := gojq.Parse("!!!")
	require.Error(t, err)

	details := ParseErrorDetails(err)
	assert.Contains(t, details, "offset")
	assert.Contains(t, details, "token")
}

func TestParseErrorDetails_EOFToken(t *testing.T) {
	// An incomplete expression produces a ParseError with an empty token (EOF).
	_, err := gojq.Parse(".")
	if err == nil {
		// "." is valid; use an expression that triggers an EOF parse error
		_, err = gojq.Parse(".foo |")
	}
	require.Error(t, err)

	details := ParseErrorDetails(err)
	assert.Contains(t, details, "<EOF>")
}

func TestParseErrorDetails_NonParseError(t *testing.T) {
	// A plain non-ParseError should return an empty string.
	details := ParseErrorDetails(fmt.Errorf("some other error"))
	assert.Empty(t, details)
}

func TestParseErrorDetails_NilError(t *testing.T) {
	details := ParseErrorDetails(nil)
	assert.Empty(t, details)
}

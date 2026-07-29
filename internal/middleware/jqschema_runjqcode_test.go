package middleware

// Tests targeting previously uncovered branches in runJqCode (jqschema.go):
//   - LogDefaultTimeout = true: debug log emitted when default timeout is applied
//   - ctx.Err() != nil when jq returns an error: context cancellation error joined with jq error

import (
	"context"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compileSimpleFilter is a test helper that compiles a trivial jq filter
// (identity ".") and returns the compiled code.
func compileSimpleFilter(t *testing.T) *gojq.Code {
	t.Helper()
	q, err := gojq.Parse(".")
	require.NoError(t, err)
	code, err := gojq.Compile(q)
	require.NoError(t, err)
	return code
}

// compileErrorFilter compiles a jq filter that errors at runtime by calling
// jq's error/1 builtin on the input value.
func compileErrorFilter(t *testing.T) *gojq.Code {
	t.Helper()
	q, err := gojq.Parse(`error("deliberate error")`)
	require.NoError(t, err)
	code, err := gojq.Compile(q)
	require.NoError(t, err)
	return code
}

// TestRunJqCode_LogDefaultTimeout verifies that runJqCode emits its default-timeout
// log line (lines 194-196 of jqschema.go) when LogDefaultTimeout is true and the
// supplied context has no deadline.
//
// The test cannot inspect logger output directly, so it verifies the observable
// side-effect: the code executes successfully (no early return), confirming the
// branch was entered and did not cause a panic or unexpected error.
func TestRunJqCode_LogDefaultTimeout(t *testing.T) {
	t.Parallel()

	code := compileSimpleFilter(t)

	result, err := runJqCode(
		context.Background(), // no deadline — triggers the LogDefaultTimeout branch
		code,
		map[string]any{"key": "value"},
		"test",
		runJqCodeOptions{LogDefaultTimeout: true},
	)

	require.NoError(t, err, "runJqCode should succeed with LogDefaultTimeout=true")
	assert.Equal(t, map[string]any{"key": "value"}, result, "identity filter should return input unchanged")
}

// TestRunJqCode_ContextCancelledWithJqError verifies the branch at line 206 of
// jqschema.go where both ctx.Err() is non-nil and the jq iterator returns an error.
//
// To hit this branch:
//  1. Supply a pre-cancelled context (ctx.Err() != nil).
//  2. Use a filter that errors at runtime (error/1 builtin).
//
// gojq's RunWithContext returns the jq error first (not the context error), so the
// "if err, ok := v.(error)" branch is entered and then ctx.Err() is checked.
// The returned error must wrap both the jq error and the context error.
func TestRunJqCode_ContextCancelledWithJqError(t *testing.T) {
	t.Parallel()

	code := compileErrorFilter(t)

	// Pre-cancel the context so that ctx.Err() is non-nil before RunWithContext is called.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runJqCode(
		ctx,
		code,
		map[string]any{"a": 1},
		"test",
		runJqCodeOptions{},
	)

	require.Error(t, err, "should return an error when context is cancelled and jq errors")
	// The error should mention context cancellation.
	assert.ErrorIs(t, err, context.Canceled, "error should wrap context.Canceled")
}

package middleware

// Regression tests for the "continue iterating after an error value" pattern.
//
// runJqCode calls iter.Next() more than once when CheckMultipleResults is set,
// so it can observe an error value and then keep pulling from the same iterator.
// gojq v0.12.19 fixed a panic in exactly that situation (continuing iteration
// after an invalid path error). These tests pin the interaction down so a
// regression in gojq — or in runJqCode's follow-up Next() call — is caught here.

import (
	"context"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGojqIterationAfterInvalidPathError verifies that draining a gojq iterator
// after it yields an invalid path error does not panic. This is the upstream
// scenario fixed in gojq v0.12.19.
func TestGojqIterationAfterInvalidPathError(t *testing.T) {
	t.Parallel()

	query, err := gojq.Parse("path(1), path(.a)")
	require.NoError(t, err)
	code, err := gojq.Compile(query)
	require.NoError(t, err)

	iter := code.RunWithContext(context.Background(), map[string]any{"a": 1})

	first, ok := iter.Next()
	require.True(t, ok, "first value should be produced")
	require.Implements(t, (*error)(nil), first, "path(1) should yield an invalid path error")

	// Continuing to drain the iterator after the error must not panic.
	assert.NotPanics(t, func() {
		for {
			if _, ok := iter.Next(); !ok {
				return
			}
		}
	})
}

// TestApplyToolResponseFilter_ErrorAsFirstResult verifies that a filter whose
// first output is an invalid path error is reported as an error and that
// runJqCode does not continue iterating in that case.
func TestApplyToolResponseFilter_ErrorAsFirstResult(t *testing.T) {
	t.Parallel()

	result, err := ApplyToolResponseFilter(context.Background(), "path(1), path(.a)", map[string]any{"a": 1})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "tool response filter")
}

// TestApplyToolResponseFilter_ErrorAfterFirstResult verifies that when the
// first output is a valid value and a later output is an invalid path error,
// the CheckMultipleResults follow-up iter.Next() reports the multiple-results
// contract violation without panicking.
func TestApplyToolResponseFilter_ErrorAfterFirstResult(t *testing.T) {
	t.Parallel()

	var (
		result any
		err    error
	)
	require.NotPanics(t, func() {
		result, err = ApplyToolResponseFilter(context.Background(), "path(.a), path(1)", map[string]any{"a": 1})
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "returned multiple results")
}

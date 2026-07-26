package logger

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogFallbackWarnings verifies that logFallbackWarnings prints two WARNING
// lines to the standard log output: one with the error message and one with the
// fallback description.
func TestLogFallbackWarnings(t *testing.T) {
	sentinel := errors.New("disk full")

	output := captureStdLog(t, func() {
		logFallbackWarnings(sentinel, "Failed to open log file", "Falling back to stderr")
	})

	assert.Contains(t, output, "WARNING:")
	assert.Contains(t, output, "Failed to open log file")
	assert.Contains(t, output, "disk full")
	assert.Contains(t, output, "Falling back to stderr")

	// Two WARNING lines must appear.
	assert.Equal(t, 2, strings.Count(output, "WARNING:"), "expected exactly two WARNING lines")
}

// TestLogFallbackWarnings_ErrorMessageInFirstLine verifies the ordering of lines:
// error message appears in the first WARNING line, fallback message in the second.
func TestLogFallbackWarnings_ErrorMessageInFirstLine(t *testing.T) {
	sentinel := errors.New("connection refused")

	output := captureStdLog(t, func() {
		logFallbackWarnings(sentinel, "Cannot reach endpoint", "Using local stub")
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "expected at least two log lines")

	// The error string and errMsg belong to the first WARNING line.
	assert.Contains(t, lines[0], "Cannot reach endpoint")
	assert.Contains(t, lines[0], "connection refused")

	// The fallback description belongs to the second WARNING line.
	assert.Contains(t, lines[1], "Using local stub")
}

// TestFallbackLoggerOnInitError_ReturnsProvidedFallback verifies that the
// function returns the fallback value with no error, regardless of the wrapped
// error severity.
func TestFallbackLoggerOnInitError_ReturnsProvidedFallback(t *testing.T) {
	fallback := "stub-logger"
	sentinel := errors.New("init failed")

	var got string
	var err error
	_ = captureStdLog(t, func() {
		got, err = fallbackLoggerOnInitError(sentinel, "Component init failed", "Using stub", fallback)
	})

	require.NoError(t, err, "fallbackLoggerOnInitError must swallow the original error")
	assert.Equal(t, fallback, got, "must return the provided fallback value unchanged")
}

// TestFallbackLoggerOnInitError_LogsWarnings verifies that the two WARNING
// lines are emitted to the standard logger when a fallback is used.
func TestFallbackLoggerOnInitError_LogsWarnings(t *testing.T) {
	sentinel := errors.New("timeout")

	output := captureStdLog(t, func() {
		_, _ = fallbackLoggerOnInitError(sentinel, "Connection timed out", "Retrying with default", 42)
	})

	assert.Contains(t, output, "Connection timed out")
	assert.Contains(t, output, "timeout")
	assert.Contains(t, output, "Retrying with default")
}

// TestFallbackLoggerOnInitError_PointerType verifies the generic behaviour with
// a pointer fallback type, matching the real usage in file_logger.go.
func TestFallbackLoggerOnInitError_PointerType(t *testing.T) {
	type myLogger struct{ name string }
	fallback := &myLogger{name: "fallback"}
	sentinel := errors.New("cannot open file")

	var got *myLogger
	_ = captureStdLog(t, func() {
		var err error
		got, err = fallbackLoggerOnInitError(sentinel, "Open failed", "Using fallback logger", fallback)
		require.NoError(t, err)
	})

	require.NotNil(t, got)
	assert.Equal(t, "fallback", got.name)
}

// TestSilentFallbackLoggerOnInitError_ReturnsProvidedFallback verifies that the
// function returns the fallback value and nil error without any log output.
func TestSilentFallbackLoggerOnInitError_ReturnsProvidedFallback(t *testing.T) {
	fallback := "silent-stub"

	output := captureStdLog(t, func() {
		got, err := silentFallbackLoggerOnInitError(fallback)
		require.NoError(t, err, "silentFallbackLoggerOnInitError must return nil error")
		assert.Equal(t, fallback, got, "must return the provided fallback value unchanged")
	})

	assert.Empty(t, output, "silentFallbackLoggerOnInitError must not emit any log output")
}

// TestSilentFallbackLoggerOnInitError_NilPointer verifies behaviour with a nil
// pointer fallback, matching the real usage with *MarkdownLogger.
func TestSilentFallbackLoggerOnInitError_NilPointer(t *testing.T) {
	type placeholder struct{}
	var fallback *placeholder // nil pointer

	output := captureStdLog(t, func() {
		got, err := silentFallbackLoggerOnInitError(fallback)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	assert.Empty(t, output, "must remain silent even for nil fallback")
}

// TestStrictLoggerOnInitError_PropagatesError verifies that the original error
// is returned unchanged and the zero value is returned for the type parameter.
func TestStrictLoggerOnInitError_PropagatesError(t *testing.T) {
	sentinel := errors.New("fatal init error")

	got, err := strictLoggerOnInitError[string](sentinel)

	require.Error(t, err)
	assert.Equal(t, sentinel, err, "must return the original error unchanged")
	assert.Equal(t, "", got, "must return the zero value for the type parameter")
}

// TestStrictLoggerOnInitError_NilErrorStillReturnsZero verifies that passing a
// nil error still returns the zero value; callers should check errors before
// relying on the returned value.
func TestStrictLoggerOnInitError_NilErrorStillReturnsZero(t *testing.T) {
	got, err := strictLoggerOnInitError[int](nil)

	assert.NoError(t, err)
	assert.Equal(t, 0, got)
}

// TestStrictLoggerOnInitError_PointerType verifies behaviour with a pointer
// type parameter, matching the real usage with *JSONLLogger.
func TestStrictLoggerOnInitError_PointerType(t *testing.T) {
	type myLogger struct{}
	sentinel := errors.New("jsonl init failed")

	got, err := strictLoggerOnInitError[*myLogger](sentinel)

	require.Error(t, err)
	assert.Equal(t, sentinel, err)
	assert.Nil(t, got, "zero value for a pointer type must be nil")
}

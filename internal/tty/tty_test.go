package tty

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"
)

// TestIsStderrTerminal verifies the function agrees with the underlying
// term.IsTerminal check for os.Stderr.
func TestIsStderrTerminal(t *testing.T) {
	expected := term.IsTerminal(int(os.Stderr.Fd()))
	result := IsStderrTerminal()
	assert.Equal(t, expected, result, "IsStderrTerminal should match term.IsTerminal(stderr)")
}

// TestIsStderrTerminal_ConsistentResult verifies that repeated calls return the same value,
// confirming deterministic behavior with no side effects.
func TestIsStderrTerminal_ConsistentResult(t *testing.T) {
	first := IsStderrTerminal()
	for i := 0; i < 5; i++ {
		assert.Equal(t, first, IsStderrTerminal(), "call %d: IsStderrTerminal should be deterministic", i+1)
	}
}

// TestIsStderrTerminal_NotATerminalInCI verifies the expected false result in automated
// environments such as CI pipelines where stderr is a pipe, not a terminal.
func TestIsStderrTerminal_NotATerminalInCI(t *testing.T) {
	if os.Getenv("CI") == "" && os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping CI-specific assertion: not running in a CI environment")
	}
	assert.False(t, IsStderrTerminal(), "stderr should not be a terminal in CI")
}

// TestIsStdoutTerminal verifies the function agrees with the underlying
// term.IsTerminal check for os.Stdout.
func TestIsStdoutTerminal(t *testing.T) {
	expected := term.IsTerminal(int(os.Stdout.Fd()))
	result := IsStdoutTerminal()
	assert.Equal(t, expected, result, "IsStdoutTerminal should match term.IsTerminal(stdout)")
}

// TestTermIsTerminal_PipeIsNotTerminal verifies that the underlying
// term.IsTerminal correctly identifies a pipe as not a terminal. This
// documents the invariant that IsStdoutTerminal and IsStderrTerminal rely on.
func TestTermIsTerminal_PipeIsNotTerminal(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()
	assert.False(t, term.IsTerminal(int(r.Fd())), "pipe file descriptor should not be a terminal")
	assert.False(t, term.IsTerminal(int(w.Fd())), "pipe write-end should not be a terminal")
}

// TestStderrTerminalWidth verifies that StderrTerminalWidth returns consistent
// results and only reports success when stderr is a terminal.
func TestStderrTerminalWidth(t *testing.T) {
	width, ok := StderrTerminalWidth()
	isTerminal := term.IsTerminal(int(os.Stderr.Fd()))
	if isTerminal {
		assert.True(t, ok, "should succeed when stderr is a terminal")
		assert.Greater(t, width, 0, "terminal width should be positive")
	} else {
		assert.False(t, ok, "should fail when stderr is not a terminal")
		assert.Equal(t, 0, width, "width should be 0 when not a terminal")
	}
}

// TestStderrTerminalWidth_NotATerminalInCI verifies that width detection
// returns false in CI where stderr is not a terminal.
func TestStderrTerminalWidth_NotATerminalInCI(t *testing.T) {
	if os.Getenv("CI") == "" && os.Getenv("GITHUB_ACTIONS") == "" {
		t.Skip("Skipping CI-specific assertion: not running in a CI environment")
	}
	width, ok := StderrTerminalWidth()
	assert.False(t, ok, "should not detect terminal width in CI")
	assert.Equal(t, 0, width, "width should be 0 in CI")
}

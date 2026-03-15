// Package tty provides utilities for TTY (terminal) detection.
// This package uses golang.org/x/term for TTY detection, which aligns with
// modern Go best practices and the spinner library v1.23.1+ implementation.
package tty

import (
	"os"

	"golang.org/x/term"
)

// IsStderrTerminal returns true if stderr is connected to a terminal.
func IsStderrTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// IsStdoutTerminal returns true if stdout is connected to a terminal.
func IsStdoutTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// StderrTerminalWidth returns the width of the terminal connected to stderr
// and true if successful. Returns 0 and false if stderr is not a terminal or
// the width cannot be determined.
func StderrTerminalWidth() (int, bool) {
	width, _, err := term.GetSize(int(os.Stderr.Fd()))
	if err != nil || width <= 0 {
		return 0, false
	}
	return width, true
}

// IsInteractiveTerminal returns true if the process is running in an
// interactive terminal context: stderr is a terminal and the process is not
// running inside a container.
func IsInteractiveTerminal() bool {
	return IsStderrTerminal() && !IsRunningInContainer()
}

// Package jqutil provides shared gojq utilities used by multiple packages.
package jqutil

import (
	"errors"
	"fmt"

	"github.com/itchyny/gojq"
)

// SecureCompileOpts are the gojq compiler options applied to every Compile call
// in this project. Centralising them here ensures the security intent ($ENV
// disabled) is never accidentally omitted from a future compile site.
//
// Both internal/config and internal/middleware need identical options. Defining
// them once in this leaf package removes the duplication and guards against the
// two copies drifting apart.
var SecureCompileOpts = []gojq.CompilerOption{
	gojq.WithEnvironLoader(func() []string { return nil }), // explicitly disable $ENV access (defense-in-depth)
}

// ParseErrorDetails returns a human-readable suffix (e.g. " at offset 3 (token "!")") when
// err wraps a *gojq.ParseError, or an empty string otherwise. EOF parse failures render
// the token as "<EOF>" for clearer diagnostics. Centralising the extraction here ensures
// that config validation and middleware filter compilation cannot diverge in their output.
func ParseErrorDetails(err error) string {
	var parseErr *gojq.ParseError
	if !errors.As(err, &parseErr) {
		return ""
	}

	token := parseErr.Token
	if token == "" {
		token = "<EOF>"
	}
	return fmt.Sprintf(" at offset %d (token %q)", parseErr.Offset, token)
}

// CompileOptsWithVariables returns a new slice combining SecureCompileOpts with
// gojq.WithVariables for the given variable names. The returned slice is always
// a fresh allocation, so callers never mutate the shared SecureCompileOpts
// backing array.
func CompileOptsWithVariables(varNames []string) []gojq.CompilerOption {
	opts := make([]gojq.CompilerOption, 0, len(SecureCompileOpts)+1)
	opts = append(opts, SecureCompileOpts...)
	opts = append(opts, gojq.WithVariables(varNames))
	return opts
}

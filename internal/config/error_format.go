package config

import (
	"errors"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/github/gh-aw-mcpg/internal/logger"
)

var logErrorFormat = logger.ForFile()

// AppendConfigDocsFooter appends standard documentation links to an error message.
func AppendConfigDocsFooter(sb *strings.Builder) {
	logErrorFormat.Print("AppendConfigDocsFooter: appending documentation links to error message")
	sb.WriteString("\n\nPlease check your configuration against the MCP Gateway specification at:")
	sb.WriteString("\n" + ConfigSpecURL)
	sb.WriteString("\n\nJSON Schema reference:")
	sb.WriteString("\n" + SchemaURL)
}

// FormatConfigError returns a rich diagnostic message for TOML parse errors.
// When err wraps a toml.ParseError, it returns ParseError.ErrorWithUsage() which
// includes a source-code snippet and column pointer, e.g.:
//
//	toml: line 5 (field command): expected "=", got "[" instead
//
//	  3 | [servers.github]
//	  4 | command = "docker"
//	  5 | [servers.github
//	      | ^
//
// For all other error types, it falls back to err.Error().
func FormatConfigError(err error) string {
	if err == nil {
		return ""
	}
	var perr toml.ParseError
	if errors.As(err, &perr) {
		logErrorFormat.Print("FormatConfigError: formatting TOML parse error with source snippet")
		return perr.ErrorWithUsage()
	}
	logErrorFormat.Printf("FormatConfigError: non-TOML-parse error, using plain message: %v", err)
	return err.Error()
}

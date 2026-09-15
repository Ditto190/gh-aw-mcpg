package mcp

import (
	"context"
	"testing"

	"github.com/github/gh-aw-mcpg/internal/sanitize"
	"github.com/stretchr/testify/assert"
)

func TestRedactRequestValueForLog(t *testing.T) {
	assert := assert.New(t)

	// Ordinary traffic keeps method-specific debug values verbatim.
	assert.Equal("repo://acme/secret/file.go", RedactRequestValueForLog(context.Background(), "repo://acme/secret/file.go"))

	// Enclave traffic reduces them to a correlatable token, so non-tool MCP methods
	// (resources/read, prompts/get) cannot bypass payload redaction.
	enclaveCtx := WithEnclaveSession(context.Background())
	redacted := RedactRequestValueForLog(enclaveCtx, "repo://acme/secret/file.go")
	assert.NotContains(redacted, "acme/secret")
	assert.Equal(sanitize.KeyedDigest("repo://acme/secret/file.go"), redacted)
	assert.Equal(redacted, RedactRequestValueForLog(enclaveCtx, "repo://acme/secret/file.go"))
}

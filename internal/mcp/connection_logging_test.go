package mcp

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogInboundRPCResponseFromResult_ReturnsResultAndError(t *testing.T) {
	assert := assert.New(t)

	expectedErr := errors.New("expected error")
	expectedResult := &Response{
		JSONRPC: "2.0",
		ID:      1,
		Result:  []byte(`{"ok":true}`),
	}

	result, err := logInboundRPCResponseFromResult("test-server", expectedResult, expectedErr, false, nil)

	assert.Same(expectedResult, result)
	assert.ErrorIs(err, expectedErr)
}

func TestLogInboundRPCResponseFromResult_AllowsNilResult(t *testing.T) {
	assert := assert.New(t)

	result, err := logInboundRPCResponseFromResult("test-server", nil, nil, false, nil)

	assert.Nil(result)
	assert.NoError(err)
}

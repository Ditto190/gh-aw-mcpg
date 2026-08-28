package hmacutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignAndVerify(t *testing.T) {
	key := []byte("test-key")
	message := "canonical message"
	signature := Sign(key, message)

	assert.Len(t, signature, 32)
	assert.True(t, Verify(key, message, signature))
	assert.False(t, Verify(key, message+" changed", signature))
	assert.False(t, Verify([]byte("other-key"), message, signature))
	assert.False(t, Verify(key, message, signature[:len(signature)-1]))
	assert.False(t, Verify(key, message, nil))
}

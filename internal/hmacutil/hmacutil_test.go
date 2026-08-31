package hmacutil

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestSignAndVerifyDebugLogging(t *testing.T) {
	if os.Getenv("GO_WANT_HMACUTIL_DEBUG_SUBPROCESS") == "1" {
		require.True(t, logHMACUtil.Enabled())

		key := []byte("private-test-key")
		message := "sensitive message"
		signature := Sign(key, message)

		assert.True(t, Verify(key, message, signature))
		assert.False(t, Verify(key, message, []byte("invalid-mac")))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestSignAndVerifyDebugLogging", "-test.v")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HMACUTIL_DEBUG_SUBPROCESS=1",
		"DEBUG=hmacutil:*",
		"DEBUG_COLORS=0",
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "subprocess output:\n%s", output)

	logOutput := string(output)
	assert.Contains(t, logOutput, "hmacutil:hmacutil Signing message: keyLen=")
	assert.Contains(t, logOutput, "hmacutil:hmacutil Signature computed: sigLen=32")
	assert.Contains(t, logOutput, "hmacutil:hmacutil Verifying signature: keyLen=")
	assert.Contains(t, logOutput, "hmacutil:hmacutil Signature verification succeeded")
	assert.Contains(t, logOutput, "hmacutil:hmacutil Signature verification failed: MAC mismatch")
	assert.NotContains(t, logOutput, "private-test-key")
	assert.NotContains(t, logOutput, "sensitive message")
}

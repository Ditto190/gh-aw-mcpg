package strutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// RandomHex returns a hex-encoded string of n cryptographically random bytes.
// The returned string has length 2*n.
func RandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate %d random bytes: %w", n, err)
	}
	return hex.EncodeToString(b), nil
}

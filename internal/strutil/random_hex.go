package strutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

// RandomHex returns a hex-encoded string of n cryptographically random bytes.
// The returned string has length 2*n.
func RandomHex(n int) (string, error) {
	if n < 0 {
		return "", fmt.Errorf("failed to generate random bytes: negative size %d", n)
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate %d random bytes: %w", n, err)
	}
	return hex.EncodeToString(b), nil
}

// RandomHexWithFallback returns a hex-encoded string of n random bytes.
// If crypto/rand is unavailable, it falls back to a process- and time-based ID
// (format: "fallback-<pid>-<nanoseconds>") that is unique within a single process run.
// The fallback is non-cryptographic and should only arise in unusual runtime environments.
func RandomHexWithFallback(n int) string {
	s, err := RandomHex(n)
	if err != nil {
		return fmt.Sprintf("fallback-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	return s
}

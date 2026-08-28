// Package hmacutil provides HMAC-SHA256 signing and verification helpers.
package hmacutil

import (
	"crypto/hmac"
	"crypto/sha256"
)

// Sign returns the HMAC-SHA256 signature for message using key.
func Sign(key []byte, message string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(message))
	return mac.Sum(nil)
}

// Verify reports whether providedMAC is a valid HMAC-SHA256 signature for message using key.
func Verify(key []byte, message string, providedMAC []byte) bool {
	return hmac.Equal(providedMAC, Sign(key, message))
}

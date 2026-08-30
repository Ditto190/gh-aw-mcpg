// Package hmacutil provides HMAC-SHA256 signing and verification helpers.
package hmacutil

import (
	"crypto/hmac"
	"crypto/sha256"

	"github.com/github/gh-aw-mcpg/internal/logger"
)

var log = logger.ForFile()

// Sign returns the HMAC-SHA256 signature for message using key.
func Sign(key []byte, message string) []byte {
	log.Printf("Signing message: keyLen=%d, messageLen=%d", len(key), len(message))
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(message))
	sum := mac.Sum(nil)
	log.Printf("Signature computed: sigLen=%d", len(sum))
	return sum
}

// Verify reports whether providedMAC is a valid HMAC-SHA256 signature for message using key.
func Verify(key []byte, message string, providedMAC []byte) bool {
	log.Printf("Verifying signature: keyLen=%d, messageLen=%d, providedMACLen=%d", len(key), len(message), len(providedMAC))
	valid := hmac.Equal(providedMAC, Sign(key, message))
	if valid {
		log.Print("Signature verification succeeded")
	} else {
		log.Print("Signature verification failed: MAC mismatch")
	}
	return valid
}

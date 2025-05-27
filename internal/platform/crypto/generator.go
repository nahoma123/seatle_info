// File: internal/platform/crypto/generator.go
package crypto

import (
	"crypto/rand"
	"encoding/base64"
)

// GenerateSecureRandomString creates a cryptographically secure random string.
// n is the number of bytes of randomness, resulting string length will be larger due to base64 encoding.
func GenerateSecureRandomString(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Package crypto provides the HMAC-based hashing helpers shared by CDKs,
// API Keys and sessions, plus opaque token generation. Raw secrets never
// reach the database; only peppered HMAC digests are stored.
package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

// HMACSHA256 returns the keyed digest of value using pepper.
func HMACSHA256(value, pepper string) []byte {
	mac := hmac.New(sha256.New, []byte(pepper))
	mac.Write([]byte(value))
	return mac.Sum(nil)
}

// NormalizeCDK uppercases the code and strips every non-alphanumeric
// character so printed codes with dashes still activate.
func NormalizeCDK(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToUpper(value) {
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

// GenerateOpaqueToken returns a 256-bit URL-safe random token.
func GenerateOpaqueToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// NewAPIKeySecret builds the full cf_live_ bearer secret shown once at
// creation time.
func NewAPIKeySecret() (string, error) {
	token, err := GenerateOpaqueToken()
	if err != nil {
		return "", err
	}
	return "cf_live_" + token, nil
}

// NewRequestID produces the platform correlation identifier prefixed req_.
func NewRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "req_unavailable"
	}
	return "req_" + hex.EncodeToString(buffer)
}

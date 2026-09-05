package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

// ErrSealedUnavailable marks ciphertext that cannot be opened (wrong master
// secret or legacy rows created before reversible storage existed).
var ErrSealedUnavailable = errors.New("sealed secret unavailable")

// deriveSealKey stretches the master secret into a 256-bit AES key. The
// master is the session secret, which already must be strong and stable in
// every environment.
func deriveSealKey(master string) []byte {
	sum := sha256.Sum256([]byte("captchaflow:sealed-secret:v1:" + master))
	return sum[:]
}

// Seal AES-256-GCM encrypts a secret so the platform can re-display it to
// its owner while the database stays useless to an attacker without the
// master secret. The random nonce is prefixed to the ciphertext.
func Seal(master, plaintext string) ([]byte, error) {
	block, err := aes.NewCipher(deriveSealKey(master))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Open reverses Seal.
func Open(master string, sealed []byte) (string, error) {
	if len(sealed) == 0 {
		return "", ErrSealedUnavailable
	}
	block, err := aes.NewCipher(deriveSealKey(master))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", ErrSealedUnavailable
	}
	plaintext, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSealedUnavailable, err)
	}
	return string(plaintext), nil
}

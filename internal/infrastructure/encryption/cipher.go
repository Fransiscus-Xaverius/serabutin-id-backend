package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	// KeySize is the AES-256 key length in bytes.
	KeySize = 32
	// fingerprintPrefix domain-separates the fingerprint from any other
	// use of the same key.
	fingerprintPrefix = "payout-account:v1:"
)

// ErrInvalidKeySize is returned when the supplied key is not exactly
// KeySize bytes long.
var ErrInvalidKeySize = errors.New("encryption key must be 32 bytes")

// Cipher encrypts short secrets with AES-256-GCM. The same value never
// encrypts to the same ciphertext (random nonce per call) and a modified
// ciphertext fails to decrypt.
type Cipher struct {
	aead  cipher.AEAD
	key   []byte
	keyID string
}

// NewCipher builds a Cipher from a raw AES-256 key. The key is copied, so
// the caller may reuse or wipe its own buffer. An empty keyID falls back to
// "v1".
func NewCipher(key []byte, keyID string) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("%w: got %d bytes", ErrInvalidKeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	if keyID == "" {
		keyID = "v1"
	}
	return &Cipher{aead: aead, key: append([]byte(nil), key...), keyID: keyID}, nil
}

// KeyID identifies the key this Cipher was built with, so a stored
// ciphertext can be matched to the key that decrypts it.
func (c *Cipher) KeyID() string { return c.keyID }

// Encrypt seals a value with a fresh random nonce and returns it as
// standard base64. The same plaintext never produces the same output twice.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	out := make([]byte, 0, len(nonce)+len(sealed))
	out = append(out, nonce...)
	out = append(out, sealed...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt opens a value produced by Encrypt. It fails on malformed input,
// on a tampered ciphertext and on a ciphertext sealed with another key.
func (c *Cipher) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext is too short")
	}
	nonce, payload := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

// Fingerprint is a keyed digest of a value: it identifies an account number
// for duplicate detection without revealing it.
func (c *Cipher) Fingerprint(plaintext string) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(fingerprintPrefix + plaintext))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

package encryption

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKey returns a deterministic 32-byte vector. It is a published test
// vector, not a credential: it exists so the suite is reproducible.
func testKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}

func TestCipher_RoundTrip(t *testing.T) {
	t.Parallel()
	c, err := NewCipher(testKey(), "v1")
	require.NoError(t, err)

	encoded, err := c.Encrypt("1234567890")
	require.NoError(t, err)
	assert.NotContains(t, encoded, "1234567890")

	plain, err := c.Decrypt(encoded)
	require.NoError(t, err)
	assert.Equal(t, "1234567890", plain)
}

func TestCipher_NonceIsUniquePerValue(t *testing.T) {
	t.Parallel()
	c, err := NewCipher(testKey(), "v1")
	require.NoError(t, err)

	first, err := c.Encrypt("1234567890")
	require.NoError(t, err)
	second, err := c.Encrypt("1234567890")
	require.NoError(t, err)
	assert.NotEqual(t, first, second, "the same value must not encrypt to the same ciphertext")
}

func TestCipher_WrongKeyCannotDecrypt(t *testing.T) {
	t.Parallel()
	c, err := NewCipher(testKey(), "v1")
	require.NoError(t, err)
	encoded, err := c.Encrypt("secret")
	require.NoError(t, err)

	otherKey := make([]byte, 32)
	for i := range otherKey {
		otherKey[i] = byte(100 + i)
	}
	other, err := NewCipher(otherKey, "v1")
	require.NoError(t, err)

	_, err = other.Decrypt(encoded)
	assert.Error(t, err)
}

func TestCipher_TamperedCiphertextFails(t *testing.T) {
	t.Parallel()
	c, err := NewCipher(testKey(), "v1")
	require.NoError(t, err)
	encoded, err := c.Encrypt("1234567890")
	require.NoError(t, err)

	// Flip one bit of the authentication tag. Mutating the base64 text
	// instead would be unreliable: the trailing characters of the last
	// group carry padding bits and can decode to the very same bytes.
	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	raw[len(raw)-1] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(raw)

	_, err = c.Decrypt(tampered)
	assert.Error(t, err)
}

func TestCipher_FingerprintIsStableAndOpaque(t *testing.T) {
	t.Parallel()
	c, err := NewCipher(testKey(), "v1")
	require.NoError(t, err)

	first := c.Fingerprint("1234567890")
	second := c.Fingerprint("1234567890")
	assert.Equal(t, first, second)
	assert.NotEqual(t, first, c.Fingerprint("9876543210"))
	assert.NotContains(t, first, "1234567890")
	assert.Len(t, first, 32)
}

func TestNewCipher_RejectsWrongKeySize(t *testing.T) {
	t.Parallel()
	_, err := NewCipher(make([]byte, 16), "v1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidKeySize)

	_, err = NewCipher(make([]byte, 64), "v1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidKeySize)

	_, err = NewCipher(nil, "v1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidKeySize)
}

func TestCipher_KeyID(t *testing.T) {
	t.Parallel()
	c, err := NewCipher(testKey(), "v2")
	require.NoError(t, err)
	assert.Equal(t, "v2", c.KeyID())

	defaulted, err := NewCipher(testKey(), "")
	require.NoError(t, err)
	assert.Equal(t, "v1", defaulted.KeyID())
}

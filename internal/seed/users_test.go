package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateUsers_ReturnsCorrectCount(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	assert.Len(t, users, 1000)
}

func TestGenerateUsers_UniqueEmails(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	emails := make(map[string]bool)
	for _, u := range users {
		assert.False(t, emails[u.Email], "duplicate email: %s", u.Email)
		emails[u.Email] = true
	}
}

func TestGenerateUsers_UniqueWhatsApp(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	phones := make(map[string]bool)
	for _, u := range users {
		assert.False(t, phones[u.WhatsApp], "duplicate whatsapp: %s", u.WhatsApp)
		phones[u.WhatsApp] = true
	}
}

func TestGenerateUsers_HashedPasswords(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	for _, u := range users {
		assert.NotEqual(t, "testing123", u.Password)
		assert.True(t, len(u.Password) > 20, "password looks unhashed: %s", u.Password)
	}
}

func TestGenerateUsers_ProviderCount(t *testing.T) {
	users, err := generateUsers(4)
	require.NoError(t, err)
	count := 0
	for _, u := range users {
		if u.Role == "provider" {
			count++
		}
	}
	assert.Equal(t, 200, count)
}

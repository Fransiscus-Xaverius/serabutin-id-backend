package entity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUser_NewFields(t *testing.T) {
	t.Parallel()
	now := time.Now()
	user := &User{
		Email:     "test@example.com",
		Password:  "hashed",
		Name:      "Test User",
		WhatsApp:  "081234567890",
		Role:      "customer",
		ConsentAt: &now,
	}

	assert.Equal(t, "081234567890", user.WhatsApp)
	assert.Equal(t, "customer", user.Role)
	assert.NotNil(t, user.ConsentAt)
	assert.Equal(t, now, *user.ConsentAt)
}

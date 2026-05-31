package dto

import (
	"testing"

	"github.com/serabutin/serabutin-id-backend/pkg/validator"
	"github.com/stretchr/testify/assert"
)

func TestRegisterRequest_Valid_ReturnsNoErrors(t *testing.T) {
	t.Parallel()
	req := RegisterRequest{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
		WhatsApp: "081234567890",
		Role:     "customer",
	}

	errors := validator.Validate(req)
	assert.Nil(t, errors)
}

func TestRegisterRequest_InvalidRole_ReturnsError(t *testing.T) {
	t.Parallel()
	req := RegisterRequest{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
		WhatsApp: "081234567890",
		Role:     "admin",
	}

	errors := validator.Validate(req)
	assert.NotNil(t, errors)
}

func TestLoginRequest_Valid_ReturnsNoErrors(t *testing.T) {
	t.Parallel()
	req := LoginRequest{
		Identifier: "test@example.com",
		Password:   "password123",
	}

	errors := validator.Validate(req)
	assert.Nil(t, errors)
}

func TestLoginRequest_MissingIdentifier_ReturnsError(t *testing.T) {
	t.Parallel()
	req := LoginRequest{
		Password: "password123",
	}

	errors := validator.Validate(req)
	assert.NotNil(t, errors)
}

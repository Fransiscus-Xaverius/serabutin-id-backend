package apperror

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDomainError_Error_ReturnsMessage(t *testing.T) {
	err := &DomainError{
		Message: "something went wrong",
	}

	result := err.Error()

	assert.Equal(t, "something went wrong", result)
}

func TestDomainError_Unwrap_ReturnsWrappedError(t *testing.T) {
	original := ErrNotFound
	err := &DomainError{
		Err:     original,
		Message: "not found",
	}

	unwrapped := err.Unwrap()

	assert.Equal(t, original, unwrapped)
}

func TestNewDomainError_CreatesWithCorrectFields(t *testing.T) {
	original := ErrValidation
	err := NewDomainError(original, "VALIDATION_FAILED", "validation failed")

	assert.Equal(t, original, err.Err)
	assert.Equal(t, "VALIDATION_FAILED", err.Code)
	assert.Equal(t, "validation failed", err.Message)
}

func TestSentinelErrors_AreNotNil(t *testing.T) {
	assert.NotNil(t, ErrNotFound)
	assert.NotNil(t, ErrDuplicateEmail)
	assert.NotNil(t, ErrInvalidCredentials)
	assert.NotNil(t, ErrTokenExpired)
	assert.NotNil(t, ErrTokenRevoked)
	assert.NotNil(t, ErrTokenInvalid)
	assert.NotNil(t, ErrInternalServer)
	assert.NotNil(t, ErrUnauthorized)
	assert.NotNil(t, ErrForbidden)
	assert.NotNil(t, ErrValidation)
}

package apperror

import (
	"errors"
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

func TestDomainError_Error_EmptyMessage(t *testing.T) {
	err := &DomainError{
		Message: "",
	}

	assert.Equal(t, "", err.Error())
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

func TestDomainError_Unwrap_WorksErrorsIs(t *testing.T) {
	err := NewDomainError(ErrNotFound, "NOT_FOUND", "resource not found")

	assert.True(t, errors.Is(err, ErrNotFound))
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestNewDomainError_CreatesWithCorrectFields(t *testing.T) {
	original := ErrValidation
	err := NewDomainError(original, "VALIDATION_FAILED", "validation failed")

	assert.Equal(t, original, err.Err)
	assert.Equal(t, "VALIDATION_FAILED", err.Code)
	assert.Equal(t, "validation failed", err.Message)
}

func TestSentinelErrors_HaveExpectedMessages(t *testing.T) {
	assert.Equal(t, "resource not found", ErrNotFound.Error())
	assert.Equal(t, "email already exists", ErrDuplicateEmail.Error())
	assert.Equal(t, "invalid email or password", ErrInvalidCredentials.Error())
	assert.Equal(t, "token has expired", ErrTokenExpired.Error())
	assert.Equal(t, "token has been revoked", ErrTokenRevoked.Error())
	assert.Equal(t, "token is invalid", ErrTokenInvalid.Error())
	assert.Equal(t, "internal server error", ErrInternalServer.Error())
	assert.Equal(t, "unauthorized", ErrUnauthorized.Error())
	assert.Equal(t, "forbidden", ErrForbidden.Error())
	assert.Equal(t, "validation failed", ErrValidation.Error())
}

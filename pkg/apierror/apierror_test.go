package apierror

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

func TestNewAPIError_SetsFields(t *testing.T) {
	t.Parallel()

	err := NewAPIError(400, "BAD_REQUEST", "something went wrong")

	assert.Equal(t, 400, err.HTTPCode)
	assert.Equal(t, "BAD_REQUEST", err.Code)
	assert.Equal(t, "something went wrong", err.Message)
}

func TestFromDomainError_NotFound_Returns404(t *testing.T) {
	t.Parallel()

	domainErr := apperror.NewDomainError(apperror.ErrNotFound, "NOT_FOUND", "resource not found")
	apiErr := FromDomainError(domainErr)

	assert.Equal(t, fiber.StatusNotFound, apiErr.HTTPCode)
	assert.Equal(t, "NOT_FOUND", apiErr.Code)
	assert.Equal(t, "resource not found", apiErr.Message)
}

func TestFromDomainError_DuplicateEmail_Returns409(t *testing.T) {
	t.Parallel()

	domainErr := apperror.NewDomainError(apperror.ErrDuplicateEmail, "DUPLICATE_EMAIL", "email already exists")
	apiErr := FromDomainError(domainErr)

	assert.Equal(t, fiber.StatusConflict, apiErr.HTTPCode)
	assert.Equal(t, "DUPLICATE_EMAIL", apiErr.Code)
	assert.Equal(t, "email already exists", apiErr.Message)
}

func TestFromDomainError_InvalidCredentials_Returns401(t *testing.T) {
	t.Parallel()

	domainErr := apperror.NewDomainError(apperror.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid email or password")
	apiErr := FromDomainError(domainErr)

	assert.Equal(t, fiber.StatusUnauthorized, apiErr.HTTPCode)
	assert.Equal(t, "INVALID_CREDENTIALS", apiErr.Code)
	assert.Equal(t, "invalid email or password", apiErr.Message)
}

func TestFromDomainError_Forbidden_Returns403(t *testing.T) {
	t.Parallel()

	domainErr := apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "access denied")
	apiErr := FromDomainError(domainErr)

	assert.Equal(t, fiber.StatusForbidden, apiErr.HTTPCode)
	assert.Equal(t, "FORBIDDEN", apiErr.Code)
	assert.Equal(t, "access denied", apiErr.Message)
}

func TestFromDomainError_Validation_Returns400(t *testing.T) {
	t.Parallel()

	domainErr := apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "validation failed")
	apiErr := FromDomainError(domainErr)

	assert.Equal(t, fiber.StatusBadRequest, apiErr.HTTPCode)
	assert.Equal(t, "VALIDATION_ERROR", apiErr.Code)
	assert.Equal(t, "validation failed", apiErr.Message)
}

func TestFromDomainError_Unknown_Returns500(t *testing.T) {
	t.Parallel()

	unknownErr := errors.New("some unknown error")
	domainErr := apperror.NewDomainError(unknownErr, "UNKNOWN", "unknown error")
	apiErr := FromDomainError(domainErr)

	assert.Equal(t, fiber.StatusInternalServerError, apiErr.HTTPCode)
	assert.Equal(t, "INTERNAL_ERROR", apiErr.Code)
	assert.Equal(t, "an unexpected error occurred", apiErr.Message)
}

func TestFromDomainError_NonDomainError_Returns500(t *testing.T) {
	t.Parallel()

	plainErr := errors.New("plain error")
	apiErr := FromDomainError(plainErr)

	assert.Equal(t, fiber.StatusInternalServerError, apiErr.HTTPCode)
	assert.Equal(t, "INTERNAL_ERROR", apiErr.Code)
	assert.Equal(t, "an unexpected error occurred", apiErr.Message)
}

func TestGlobalErrorHandler_APIError_ReturnsCorrectJSON(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{
		ErrorHandler: GlobalErrorHandler,
	})

	app.Get("/test", func(c fiber.Ctx) error {
		return NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "invalid input")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "test-request-id")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)

	var apiErr APIError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&apiErr))
	assert.Equal(t, "VALIDATION_ERROR", apiErr.Code)
	assert.Equal(t, "invalid input", apiErr.Message)
}

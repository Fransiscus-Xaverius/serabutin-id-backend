package apierror

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

type APIError struct {
	HTTPCode  int         `json:"-"`
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	RequestID string      `json:"request_id,omitempty"`
	Details   interface{} `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return e.Message
}

func NewAPIError(httpCode int, code, message string) *APIError {
	return &APIError{
		HTTPCode: httpCode,
		Code:     code,
		Message:  message,
	}
}

func FromDomainError(err error) *APIError {
	var domainErr *apperror.DomainError
	if errors.As(err, &domainErr) {
		switch {
		case errors.Is(domainErr.Unwrap(), apperror.ErrNotFound):
			return NewAPIError(fiber.StatusNotFound, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrDuplicateEmail):
			return NewAPIError(fiber.StatusConflict, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrInvalidCredentials):
			return NewAPIError(fiber.StatusUnauthorized, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrTokenExpired):
			return NewAPIError(fiber.StatusUnauthorized, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrTokenRevoked):
			return NewAPIError(fiber.StatusUnauthorized, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrTokenInvalid):
			return NewAPIError(fiber.StatusUnauthorized, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrUnauthorized):
			return NewAPIError(fiber.StatusUnauthorized, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrForbidden):
			return NewAPIError(fiber.StatusForbidden, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrValidation):
			return NewAPIError(fiber.StatusBadRequest, domainErr.Code, domainErr.Message)
		default:
			return NewAPIError(fiber.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
		}
	}

	return NewAPIError(fiber.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
}

func GlobalErrorHandler(c fiber.Ctx, err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		apiErr.RequestID = requestctx.GetRequestID(c)
		return c.Status(apiErr.HTTPCode).JSON(apiErr)
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return c.Status(fiberErr.Code).JSON(&APIError{
			Code:      "REQUEST_ERROR",
			Message:   fiberErr.Message,
			RequestID: requestctx.GetRequestID(c),
		})
	}

	return c.Status(fiber.StatusInternalServerError).JSON(&APIError{
		Code:      "INTERNAL_ERROR",
		Message:   "an unexpected error occurred",
		RequestID: requestctx.GetRequestID(c),
	})
}

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
		// The admin moderation state machine reports disallowed transitions
		// with this code; it maps to 422 while the legacy INVALID_TRANSITION
		// codes used by booking/bid flows stay at 400.
		if domainErr.Code == "INVALID_STATUS_TRANSITION" {
			return NewAPIError(fiber.StatusUnprocessableEntity, domainErr.Code, domainErr.Message)
		}
		// The service taxonomy rejects unknown categories with this code; it
		// maps to 422 (like the admin moderation state machine) so clients
		// can distinguish "unknown category" from generic 400s.
		if domainErr.Code == "UNKNOWN_CATEGORY" {
			return NewAPIError(fiber.StatusUnprocessableEntity, domainErr.Code, domainErr.Message)
		}
		// A payout destination with an invalid bank code or account number
		// maps to 422. A malformed request body stays 400, and the global
		// ErrValidation mapping is unchanged for other endpoints.
		if domainErr.Code == "INVALID_PAYOUT_ACCOUNT" {
			return NewAPIError(fiber.StatusUnprocessableEntity, domainErr.Code, domainErr.Message)
		}
		// The admin chat-safety proxy reports a down/unconfigured chat
		// backend with this code; it maps to 502 while keeping the code so
		// operators can distinguish it from local 500s.
		if domainErr.Code == "CHAT_DIRECTORY_UNAVAILABLE" {
			return NewAPIError(fiber.StatusBadGateway, domainErr.Code, domainErr.Message)
		}
		// Ticket 12: kill-switch gates report a disabled module with this
		// code; it maps to 503 so clients can distinguish a freeze from a
		// generic 400, with the module named in the message.
		if domainErr.Code == "MODULE_DISABLED" {
			return NewAPIError(fiber.StatusServiceUnavailable, domainErr.Code, domainErr.Message)
		}
		switch {
		case errors.Is(domainErr.Unwrap(), apperror.ErrNotFound):
			return NewAPIError(fiber.StatusNotFound, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrServiceNotFound):
			return NewAPIError(fiber.StatusNotFound, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrBookingNotFound):
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
		case errors.Is(domainErr.Unwrap(), apperror.ErrAccountSuspended):
			return NewAPIError(fiber.StatusForbidden, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrNotServiceOwner):
			return NewAPIError(fiber.StatusForbidden, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrNotBookingParticipant):
			return NewAPIError(fiber.StatusForbidden, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrValidation):
			return NewAPIError(fiber.StatusBadRequest, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrServiceInactive):
			return NewAPIError(fiber.StatusBadRequest, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrInvalidTransition):
			return NewAPIError(fiber.StatusBadRequest, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrPaymentRequired):
			return NewAPIError(fiber.StatusPaymentRequired, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrPaymentNotConfigured):
			return NewAPIError(fiber.StatusServiceUnavailable, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrReviewAlreadyExists):
			return NewAPIError(fiber.StatusConflict, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrAlreadyExists):
			return NewAPIError(fiber.StatusConflict, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrBookingNotCompleted):
			return NewAPIError(fiber.StatusBadRequest, domainErr.Code, domainErr.Message)
		case errors.Is(domainErr.Unwrap(), apperror.ErrNotBookingOwner):
			return NewAPIError(fiber.StatusForbidden, domainErr.Code, domainErr.Message)
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

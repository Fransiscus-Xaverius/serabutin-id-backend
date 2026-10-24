package handler

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/payment"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

// PaymentHandler serves the prepayment surface: the public booking config,
// the customer's payment status, DOKU's notification callback, and the
// local-only mock endpoints used to exercise the flow without credentials.
type PaymentHandler struct {
	paymentUsecase *usecase.PaymentUsecase
	platformFee    int64
}

func NewPaymentHandler(paymentUsecase *usecase.PaymentUsecase, platformFee int64) *PaymentHandler {
	return &PaymentHandler{
		paymentUsecase: paymentUsecase,
		platformFee:    platformFee,
	}
}

// GetConfig publishes the values the customer needs before paying: the
// platform fee that will be added to the total and the payment channel that
// is currently supported.
func (h *PaymentHandler) GetConfig(c fiber.Ctx) error {
	return response.Success(c, fiber.StatusOK, dto.BookingConfigResponse{
		PlatformFee:          h.platformFee,
		PaymentExpiryMinutes: h.paymentUsecase.ExpiryMinutes(),
		PaymentMethods:       []string{entity.PaymentChannelQRIS},
	})
}

// GetPayment returns the payment state of one booking to its participants.
func (h *PaymentHandler) GetPayment(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid booking ID"))
	}

	record, err := h.paymentUsecase.GetForBooking(c.Context(), userID, uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return response.Success(c, fiber.StatusOK, toPaymentResponse(record))
}

// Webhook receives DOKU's HTTP notification. It is public because DOKU
// calls it server to server, and the signature is what makes it trusted.
func (h *PaymentHandler) Webhook(c fiber.Ctx) error {
	raw := c.Body()
	headers := map[string]string{
		"Client-Id":         c.Get("Client-Id"),
		"Request-Id":        c.Get("Request-Id"),
		"Request-Timestamp": c.Get("Request-Timestamp"),
		"Signature":         c.Get("Signature"),
	}

	if _, err := h.paymentUsecase.ApplyNotification(c.Context(), raw, headers); err != nil {
		if errors.Is(err, payment.ErrInvalidSignature) {
			return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "INVALID_SIGNATURE", "invalid notification signature"))
		}
		if isPaymentNotFound(err) {
			// DOKU retries notifications it could not confirm. An invoice we
			// do not know about is acknowledged so the retry storm stops.
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "IGNORED"})
		}
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "SUCCESS"})
}

// MockSucceed simulates a successful QRIS payment. It only exists while the
// deployment runs the mock gateway.
func (h *PaymentHandler) MockSucceed(c fiber.Ctx) error {
	return h.mockStatus(c, "SUCCESS")
}

// MockExpire simulates an expired QRIS payment.
func (h *PaymentHandler) MockExpire(c fiber.Ctx) error {
	return h.mockStatus(c, "EXPIRED")
}

func (h *PaymentHandler) mockStatus(c fiber.Ctx, status string) error {
	if !h.paymentUsecase.MockMode() {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusNotFound, "NOT_FOUND", "mock payment endpoints are disabled"))
	}

	invoice := strings.TrimSpace(c.Params("invoice"))
	if invoice == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_INVOICE", "invoice number is required"))
	}

	record, err := h.paymentUsecase.ApplyMockStatus(c.Context(), invoice, status)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return response.Success(c, fiber.StatusOK, toPaymentResponse(record))
}

// toPaymentResponse maps a payment entity to its API shape.
func toPaymentResponse(record *entity.Payment) *dto.PaymentResponse {
	if record == nil {
		return nil
	}

	resp := &dto.PaymentResponse{
		Status:        record.Status,
		Channel:       record.Channel,
		InvoiceNumber: record.InvoiceNumber,
		PaymentURL:    record.PaymentURL,
		Amount:        record.Amount,
	}
	if record.ExpiresAt != nil {
		s := record.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z")
		resp.ExpiresAt = &s
	}
	if record.PaidAt != nil {
		s := record.PaidAt.UTC().Format("2006-01-02T15:04:05Z")
		resp.PaidAt = &s
	}
	return resp
}

func isPaymentNotFound(err error) bool {
	var domainErr *apperror.DomainError
	if errors.As(err, &domainErr) {
		return errors.Is(domainErr.Unwrap(), apperror.ErrNotFound)
	}
	return false
}

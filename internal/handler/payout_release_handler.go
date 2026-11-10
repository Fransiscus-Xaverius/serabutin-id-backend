package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

// transferWebhookPath is the path DOKU posts Kirim DOKU notifications to.
const transferWebhookPath = "/api/v1/payments/doku/transfer-webhook"

// payoutReleaser is the part of the payout usecase the release handler needs.
type payoutReleaser interface {
	Release(ctx context.Context, bookingID uint) (*entity.Payout, error)
	SyncTransferStatus(ctx context.Context, partnerReferenceNo string) (*entity.Payout, error)
	SyncRefundStatus(ctx context.Context, transactionID string) (*entity.Payout, error)
}

// PayoutReleaseHandler serves the admin payout release and the Kirim DOKU
// notification. Only the release moves money, and only through the usecase.
type PayoutReleaseHandler struct {
	payouts    payoutReleaser
	agentKey   string
	releaseErr error
}

// NewPayoutReleaseHandler takes releaseErr from config.RequirePayoutRelease:
// while it is set the release is refused, but the notification still triggers
// a check-status for transfers DOKU has already made.
func NewPayoutReleaseHandler(payouts payoutReleaser, agentKey string, releaseErr error) *PayoutReleaseHandler {
	return &PayoutReleaseHandler{payouts: payouts, agentKey: agentKey, releaseErr: releaseErr}
}

// Release pays the Mitra of one booking. A repeated call returns the same
// payout state and moves no money again.
func (h *PayoutReleaseHandler) Release(c fiber.Ctx) error {
	if h.releaseErr != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusServiceUnavailable, "PAYOUT_RELEASE_NOT_CONFIGURED", h.releaseErr.Error()))
	}

	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid booking ID"))
	}

	payout, err := h.payouts.Release(c.Context(), uint(id))
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, toPayoutReleaseResponse(payout))
}

// transferNotification reads the identifiers for the agreement and the ack.
// transactionStatus is echoed back to DOKU but never applied: the final state
// always comes from the SNAP check-status call.
type transferNotification struct {
	TransactionID     string `json:"transactionId"`
	InvoiceNumber     string `json:"invoiceNumber"`
	SendTrxID         string `json:"sendTrxId"`
	TransactionStatus string `json:"transactionStatus"`
}

// TransferWebhook receives a Kirim DOKU notification. It is advisory: it
// names a payout, and the outcome comes from the SNAP check-status API, never
// from the body. It never moves money.
//
// Kirim DOKU publishes requestId, agentKey and signature headers. The public
// documentation does not describe the signature scheme, so this handler only
// checks agentKey and requestId and fails closed when the agent key is unset.
// Polling is the source of truth; this endpoint only shortens the wait.
func (h *PayoutReleaseHandler) TransferWebhook(c fiber.Ctx) error {
	if h.agentKey == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusServiceUnavailable, "PAYOUT_NOTIFICATION_NOT_CONFIGURED", "DOKU_PAYOUT_NOTIFICATION_AGENT_KEY must be set"))
	}
	agentKey := c.Get("agentKey")
	if agentKey == "" || subtle.ConstantTimeCompare([]byte(agentKey), []byte(h.agentKey)) != 1 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "INVALID_NOTIFICATION", "invalid notification agent key"))
	}
	if strings.TrimSpace(c.Get("requestId")) == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "INVALID_NOTIFICATION", "notification has no requestId"))
	}

	var notification transferNotification
	if err := json.Unmarshal(c.Body(), &notification); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_NOTIFICATION", "notification body is not readable, nothing was changed"))
	}
	// DOKU has used both fields for the partner reference across versions, so
	// try each distinct value. Only a reference the payout store does not know
	// moves to the next candidate.
	references := distinctReferences(notification.InvoiceNumber, notification.SendTrxID)
	transactionID := strings.TrimSpace(notification.TransactionID)
	if len(references) == 0 && transactionID == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_NOTIFICATION", "notification has no transactionId, invoiceNumber, or sendTrxId, nothing was changed"))
	}
	for _, reference := range references {
		_, err := h.payouts.SyncTransferStatus(c.Context(), reference)
		if err == nil {
			return c.Status(fiber.StatusOK).JSON(payoutNotificationAck(notification))
		}
		if !errors.Is(err, apperror.ErrNotFound) {
			return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
		}
	}
	// Refund notifications carry only transactionId. Resolve it against DOKU's
	// own reference and let check-status decide; a paid payout is reopened only
	// when DOKU reports refunded or failed.
	if transactionID != "" {
		_, err := h.payouts.SyncRefundStatus(c.Context(), transactionID)
		if err != nil && !errors.Is(err, apperror.ErrNotFound) {
			return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
		}
	}
	// DOKU retries notifications it could not confirm. A transfer we do not
	// know about is acknowledged so the retry storm stops; polling remains the
	// source of truth.
	return c.Status(fiber.StatusOK).JSON(payoutNotificationAck(notification))
}

// distinctReferences returns the non-blank identifiers in order, without
// duplicates, so the same value is not checked twice.
func distinctReferences(values ...string) []string {
	seen := map[string]struct{}{}
	references := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		references = append(references, value)
	}
	return references
}

// payoutNotificationAck is the acknowledgement Kirim DOKU documents. The
// status is always true: check-status, not this body, decides the payout.
func payoutNotificationAck(notification transferNotification) fiber.Map {
	return fiber.Map{
		"status":          true,
		"transactionId":   notification.TransactionID,
		"responseCode":    "00",
		"responseMessage": "Successfully processed",
	}
}

// toPayoutReleaseResponse is the only shape a payout leaves the API in: the
// destination is reduced to its bank code and last four digits.
func toPayoutReleaseResponse(payout *entity.Payout) dto.PayoutReleaseResponse {
	return dto.PayoutReleaseResponse{
		ID:                 payout.ID,
		BookingID:          payout.BookingID,
		Status:             payout.Status,
		Amount:             payout.Amount,
		Currency:           payout.Currency,
		PartnerReferenceNo: payout.PartnerReferenceNo,
		BankCode:           payout.BeneficiaryBankCode,
		AccountLast4:       payout.BeneficiaryLast4,
		Replayed:           payout.Replayed,
	}
}

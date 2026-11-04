package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

// transferWebhookPath is the path DOKU posts transfer notifications to. The
// Sub-Account signature covers it, so it must match the URL set in the dashboard.
const transferWebhookPath = "/api/v1/payments/doku/transfer-webhook"

// payoutReleaser is the part of the payout usecase the release handler needs.
type payoutReleaser interface {
	Release(ctx context.Context, bookingID uint) (*entity.Payout, error)
	SyncTransferStatus(ctx context.Context, partnerReferenceNo string) (*entity.Payout, error)
}

// PayoutReleaseHandler serves the admin payout release and DOKU's transfer
// notification. Only the release moves money, and only through the usecase.
type PayoutReleaseHandler struct {
	payouts    payoutReleaser
	secretKey  string
	releaseErr error
}

// NewPayoutReleaseHandler takes releaseErr from config.RequirePayoutRelease:
// while it is set the release is refused, but the notification still records
// the outcome of transfers DOKU has already made.
func NewPayoutReleaseHandler(payouts payoutReleaser, secretKey string, releaseErr error) *PayoutReleaseHandler {
	return &PayoutReleaseHandler{payouts: payouts, secretKey: secretKey, releaseErr: releaseErr}
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

// transferNotification reads only the partner reference, the key that routes
// the notification to a payout. Nothing else in the body is trusted.
type transferNotification struct {
	PartnerReferenceNo string `json:"partnerReferenceNo"`
}

// TransferWebhook receives DOKU's transfer notification. It is advisory: it
// names a payout, and the outcome comes from DOKU's transactions-status API,
// never from the body. It never moves money.
//
// ASSUMPTION, NOT DOCUMENTED BY DOKU: the notification's JSON fields and the
// header names read below (Authorization bearer token, X-SIGNATURE or
// Signature, X-TIMESTAMP or Request-Timestamp) are not in DOKU's public
// Sub-Account V2 docs. They must be confirmed against a real sandbox
// notification before go-live. The signature scheme itself is documented.
func (h *PayoutReleaseHandler) TransferWebhook(c fiber.Ctx) error {
	raw := c.Body()
	token, ok := bearerToken(c.Get("Authorization"))
	signature := firstHeader(c, "X-SIGNATURE", "Signature")
	timestamp := firstHeader(c, "X-TIMESTAMP", "Request-Timestamp")
	if !ok || !verifyTransferSignature(h.secretKey, token, timestamp, signature, raw) {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "INVALID_SIGNATURE", "invalid notification signature"))
	}

	var notification transferNotification
	if err := json.Unmarshal(raw, &notification); err != nil || notification.PartnerReferenceNo == "" {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_NOTIFICATION", "notification has no readable partner reference, nothing was changed"))
	}

	if _, err := h.payouts.SyncTransferStatus(c.Context(), notification.PartnerReferenceNo); err != nil {
		if errors.Is(err, apperror.ErrNotFound) {
			// DOKU retries notifications it could not confirm. A transfer we
			// do not know about is acknowledged so the retry storm stops.
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "IGNORED"})
		}
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "SUCCESS"})
}

// verifyTransferSignature checks the Sub-Account signature over the
// notification path and the raw body. An empty secret or any missing part
// fails closed.
func verifyTransferSignature(secretKey, token, timestamp, signature string, body []byte) bool {
	if secretKey == "" || token == "" || timestamp == "" || signature == "" {
		return false
	}
	expected := doku.SignSHA512(secretKey, http.MethodPost, transferWebhookPath, token, string(body), timestamp)
	return subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) == 1
}

// firstHeader returns the value of the first named header that is set.
func firstHeader(c fiber.Ctx, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(c.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func bearerToken(header string) (string, bool) {
	token, found := strings.CutPrefix(header, "Bearer ")
	token = strings.TrimSpace(token)
	return token, found && token != ""
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

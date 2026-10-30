package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
	"github.com/serabutin/serabutin-id-backend/internal/middleware"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

const (
	releaseTestSecret    = "doku-secret-key-for-tests"
	releaseTestToken     = "b2b-token-for-tests"
	releaseTestTimestamp = "2026-10-31T10:00:00+07:00"
	transferBodyPaid     = `{"partnerReferenceNo":"payout-booking-42","latestTransactionStatus":"00"}`
)

func TestPayoutReleaseHandler_AdminReleasesAndGetsPayoutState(t *testing.T) {
	uc := &fakePayoutReleaser{release: &entity.Payout{
		ID: 5, BookingID: 42, Status: entity.PayoutStatusSubmitted, Amount: 150000, Currency: "IDR",
		PartnerReferenceNo: "payout-booking-42", BeneficiaryBankCode: "BCAIDJA",
		BeneficiaryLast4: "7890", BeneficiaryAccountNumberEncrypted: "enc-secret",
	}}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "admin"}, 1, releaseTestSecret, nil)

	resp := postRelease(t, app, "/api/v1/admin/bookings/42/payout")
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, uint(42), uc.releasedBookingID)
	assert.Contains(t, body, `"status":"submitted"`)
	assert.Contains(t, body, `"partner_reference_no":"payout-booking-42"`)
	assert.Contains(t, body, `"account_last4":"7890"`)
	assert.NotContains(t, body, "enc-secret")
}

func TestPayoutReleaseHandler_RepeatedReleaseReturnsSamePayout(t *testing.T) {
	uc := &fakePayoutReleaser{release: &entity.Payout{ID: 5, BookingID: 42, Status: entity.PayoutStatusPaid, Amount: 150000}}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "admin"}, 1, releaseTestSecret, nil)

	first := postRelease(t, app, "/api/v1/admin/bookings/42/payout")
	defer first.Body.Close()
	second := postRelease(t, app, "/api/v1/admin/bookings/42/payout")
	defer second.Body.Close()

	firstRaw, _ := io.ReadAll(first.Body)
	secondRaw, _ := io.ReadAll(second.Body)
	assert.Equal(t, 200, first.StatusCode)
	assert.Equal(t, 200, second.StatusCode)
	assert.Equal(t, string(firstRaw), string(secondRaw))
	assert.Equal(t, 2, uc.releaseCalls)
}

func TestPayoutReleaseHandler_NonAdminIsForbiddenAndNeverReleases(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "provider"}, 7, releaseTestSecret, nil)

	resp := postRelease(t, app, "/api/v1/admin/bookings/42/payout")
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode)
	assert.Equal(t, 0, uc.releaseCalls, "a non-admin must never reach the usecase")
}

func TestPayoutReleaseHandler_RefusedWhileNotConfigured(t *testing.T) {
	uc := &fakePayoutReleaser{}
	releaseErr := apperror.NewDomainError(apperror.ErrValidation, "X", "payout release is not configured: DOKU_SOURCE_ACCOUNT must be set")
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "admin"}, 1, releaseTestSecret, releaseErr)

	resp := postRelease(t, app, "/api/v1/admin/bookings/42/payout")
	defer resp.Body.Close()

	assert.Equal(t, 503, resp.StatusCode)
	assert.Equal(t, "PAYOUT_RELEASE_NOT_CONFIGURED", errorCode(t, resp))
	assert.Equal(t, 0, uc.releaseCalls, "an unconfigured deployment must never reach the usecase")
}

func TestPayoutReleaseHandler_ReleaseErrorsMapToTheirStatus(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
		code   string
	}{
		"booking not found": {apperror.NewDomainError(apperror.ErrNotFound, "BOOKING_NOT_FOUND", "booking not found"), 404, "BOOKING_NOT_FOUND"},
		"not releasable":    {apperror.NewDomainError(apperror.ErrValidation, "PAYOUT_BOOKING_NOT_RELEASABLE", "booking must be confirmed or completed before its payout"), 400, "PAYOUT_BOOKING_NOT_RELEASABLE"},
		"transfer failed":   {apperror.NewDomainError(apperror.ErrInternalServer, "PAYOUT_TRANSFER_FAILED", "transfer inquiry failed, retry the release"), 500, "INTERNAL_ERROR"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			uc := &fakePayoutReleaser{releaseErr: tc.err}
			app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "admin"}, 1, releaseTestSecret, nil)

			resp := postRelease(t, app, "/api/v1/admin/bookings/42/payout")
			defer resp.Body.Close()

			assert.Equal(t, tc.status, resp.StatusCode)
			assert.Equal(t, tc.code, errorCode(t, resp))
		})
	}
}

func TestPayoutReleaseHandler_InvalidBookingIDIs400(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "admin"}, 1, releaseTestSecret, nil)

	resp := postRelease(t, app, "/api/v1/admin/bookings/abc/payout")
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, 0, uc.releaseCalls)
}

func TestPayoutReleaseHandler_TransferWebhookWithValidSignatureRecordsStatus(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhook(t, app, transferBodyPaid, transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid))
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(raw), "SUCCESS")
	assert.Equal(t, []string{"payout-booking-42:paid"}, uc.appliedStatuses)
	assert.Equal(t, 0, uc.releaseCalls, "a webhook must never move money")
}

func TestPayoutReleaseHandler_RepeatedTransferWebhookIsForwardedWithoutChange(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)
	signature := transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid)

	first := postTransferWebhook(t, app, transferBodyPaid, signature)
	defer first.Body.Close()
	second := postTransferWebhook(t, app, transferBodyPaid, signature)
	defer second.Body.Close()

	assert.Equal(t, 200, first.StatusCode)
	assert.Equal(t, 200, second.StatusCode)
	assert.Equal(t, []string{"payout-booking-42:paid", "payout-booking-42:paid"}, uc.appliedStatuses)
}

func TestPayoutReleaseHandler_TransferWebhookStatusMapping(t *testing.T) {
	cases := map[string]string{
		"00": "paid",
		"03": "submitted",
		"06": "failed",
		"05": "failed",
		"04": "failed",
	}
	for code, want := range cases {
		t.Run(code, func(t *testing.T) {
			body := `{"partnerReferenceNo":"payout-booking-42","latestTransactionStatus":"` + code + `"}`
			uc := &fakePayoutReleaser{}
			app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

			resp := postTransferWebhook(t, app, body, transferSignature(releaseTestSecret, transferWebhookPath, body))
			defer resp.Body.Close()

			assert.Equal(t, 200, resp.StatusCode)
			assert.Equal(t, []string{"payout-booking-42:" + want}, uc.appliedStatuses)
		})
	}
}

func TestPayoutReleaseHandler_TransferWebhookRejectsBadSignature(t *testing.T) {
	const tamperedBody = `{"partnerReferenceNo":"payout-booking-99","latestTransactionStatus":"00"}`
	cases := map[string]struct {
		signature string
		body      string
		token     string
	}{
		"signed with another secret":    {transferSignature("other-secret", transferWebhookPath, transferBodyPaid), transferBodyPaid, releaseTestToken},
		"signed over the checkout path": {transferSignature(releaseTestSecret, "/api/v1/payments/doku/webhook", transferBodyPaid), transferBodyPaid, releaseTestToken},
		"body tampered after signing":   {transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid), tamperedBody, releaseTestToken},
		"token not the signed one":      {transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid), transferBodyPaid, "another-token"},
		"signature missing":             {"", transferBodyPaid, releaseTestToken},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			uc := &fakePayoutReleaser{}
			app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

			resp := postTransferWebhookWithHeaders(t, app, tc.body, tc.signature, tc.token, releaseTestTimestamp)
			defer resp.Body.Close()

			assert.Equal(t, 401, resp.StatusCode)
			assert.Equal(t, "INVALID_SIGNATURE", errorCode(t, resp))
			assert.Empty(t, uc.appliedStatuses, "an unverifiable notification must change nothing")
		})
	}
}

func TestPayoutReleaseHandler_TransferWebhookFailsClosedWithoutSecret(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, "", nil)

	resp := postTransferWebhook(t, app, transferBodyPaid, transferSignature("", transferWebhookPath, transferBodyPaid))
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
	assert.Empty(t, uc.appliedStatuses)
}

func TestPayoutReleaseHandler_TransferWebhookWithoutBearerTokenIs401(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhookWithHeaders(t, app, transferBodyPaid, transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid), "", releaseTestTimestamp)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
	assert.Empty(t, uc.appliedStatuses)
}

func TestPayoutReleaseHandler_TransferWebhookErrorsNeverEchoCredentials(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhookWithHeaders(t, app, transferBodyPaid, "bad-signature", releaseTestToken, releaseTestTimestamp)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	assert.NotContains(t, string(raw), releaseTestSecret)
	assert.NotContains(t, string(raw), releaseTestToken)
}

func TestPayoutReleaseHandler_TransferWebhookUnknownStatusIs400(t *testing.T) {
	body := `{"partnerReferenceNo":"payout-booking-42","latestTransactionStatus":"99"}`
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhook(t, app, body, transferSignature(releaseTestSecret, transferWebhookPath, body))
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, "INVALID_TRANSACTION_STATUS", errorCode(t, resp))
	assert.Empty(t, uc.appliedStatuses)
}

func TestPayoutReleaseHandler_TransferWebhookMissingReferenceIs400(t *testing.T) {
	body := `{"latestTransactionStatus":"00"}`
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhook(t, app, body, transferSignature(releaseTestSecret, transferWebhookPath, body))
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, "INVALID_NOTIFICATION", errorCode(t, resp))
}

func TestPayoutReleaseHandler_TransferWebhookUnknownTransferIsAcknowledged(t *testing.T) {
	uc := &fakePayoutReleaser{applyErr: apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_NOT_FOUND", "payout not found")}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhook(t, app, transferBodyPaid, transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid))
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(raw), "IGNORED")
}

// transferSignature signs a notification exactly as DOKU does: the Sub-Account
// scheme over the method, the path, the bearer token, the body and the timestamp.
func transferSignature(secret, path, body string) string {
	return doku.SignSHA512(secret, http.MethodPost, path, releaseTestToken, body, releaseTestTimestamp)
}

func postTransferWebhook(t *testing.T, app *fiber.App, body, signature string) *http.Response {
	t.Helper()
	return postTransferWebhookWithHeaders(t, app, body, signature, releaseTestToken, releaseTestTimestamp)
}

func postTransferWebhookWithHeaders(t *testing.T, app *fiber.App, body, signature, token, timestamp string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/payments/doku/transfer-webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if timestamp != "" {
		req.Header.Set("X-TIMESTAMP", timestamp)
	}
	if signature != "" {
		req.Header.Set("X-SIGNATURE", signature)
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	return resp
}

func postRelease(t *testing.T, app *fiber.App, path string) *http.Response {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("POST", path, nil))
	require.NoError(t, err)
	return resp
}

func newReleaseTestApp(uc *fakePayoutReleaser, profiles *fakeProfileUsecase, userID uint, secret string, releaseErr error) *fiber.App {
	h := NewPayoutReleaseHandler(uc, secret, releaseErr)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, userID)
		return c.Next()
	})
	app.Post("/api/v1/admin/bookings/:id/payout", middleware.RequireAdmin(profiles), h.Release)
	app.Post("/api/v1/payments/doku/transfer-webhook", h.TransferWebhook)
	return app
}

type fakePayoutReleaser struct {
	release           *entity.Payout
	releaseErr        error
	applyErr          error
	releaseCalls      int
	releasedBookingID uint
	appliedStatuses   []string
}

func (f *fakePayoutReleaser) Release(ctx context.Context, bookingID uint) (*entity.Payout, error) {
	f.releaseCalls++
	f.releasedBookingID = bookingID
	if f.releaseErr != nil {
		return nil, f.releaseErr
	}
	if f.release == nil {
		return &entity.Payout{BookingID: bookingID}, nil
	}
	return f.release, nil
}

func (f *fakePayoutReleaser) ApplyTransferStatus(ctx context.Context, partnerReferenceNo, status string) error {
	f.appliedStatuses = append(f.appliedStatuses, partnerReferenceNo+":"+status)
	return f.applyErr
}

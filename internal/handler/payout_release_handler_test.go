package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/doku"
	"github.com/serabutin/serabutin-id-backend/internal/middleware"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

const (
	releaseTestSecret    = "doku-secret-key-for-tests"
	releaseTestToken     = "b2b-token-for-tests"
	releaseTestTimestamp = "2026-10-31T10:00:00+07:00"
	releaseTestRef       = "payout-booking-42"
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

func TestPayoutReleaseHandler_TransferWebhookWithValidSignatureSyncsThePayout(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhook(t, app, transferBodyPaid, transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid))
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(raw), "SUCCESS")
	assert.Equal(t, []string{releaseTestRef}, uc.syncedRefs)
	assert.Equal(t, 0, uc.releaseCalls, "a webhook must never move money")
}

func TestPayoutReleaseHandler_TransferWebhookPassesOnlyTheReferenceNotTheBodyStatus(t *testing.T) {
	// The body claims success or failure; the handler must ignore that claim
	// and hand the usecase only the partner reference.
	for _, code := range []string{"00", "06", "99"} {
		t.Run(code, func(t *testing.T) {
			body := `{"partnerReferenceNo":"payout-booking-42","latestTransactionStatus":"` + code + `"}`
			uc := &fakePayoutReleaser{}
			app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

			resp := postTransferWebhook(t, app, body, transferSignature(releaseTestSecret, transferWebhookPath, body))
			defer resp.Body.Close()

			assert.Equal(t, 200, resp.StatusCode)
			assert.Equal(t, []string{releaseTestRef}, uc.syncedRefs)
		})
	}
}

func TestPayoutReleaseHandler_TransferWebhookRepeatedNotificationSyncsAgain(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)
	signature := transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid)

	first := postTransferWebhook(t, app, transferBodyPaid, signature)
	defer first.Body.Close()
	second := postTransferWebhook(t, app, transferBodyPaid, signature)
	defer second.Body.Close()

	assert.Equal(t, 200, first.StatusCode)
	assert.Equal(t, 200, second.StatusCode)
	assert.Equal(t, []string{releaseTestRef, releaseTestRef}, uc.syncedRefs)
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
			assert.Empty(t, uc.syncedRefs, "an unverifiable notification must change nothing")
		})
	}
}

func TestPayoutReleaseHandler_TransferWebhookFailsClosedWithoutSecret(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, "", nil)

	resp := postTransferWebhook(t, app, transferBodyPaid, transferSignature("", transferWebhookPath, transferBodyPaid))
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
	assert.Empty(t, uc.syncedRefs)
}

func TestPayoutReleaseHandler_TransferWebhookWithoutBearerTokenIs401(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhookWithHeaders(t, app, transferBodyPaid, transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid), "", releaseTestTimestamp)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
	assert.Empty(t, uc.syncedRefs)
}

func TestPayoutReleaseHandler_TransferWebhookAcceptsTheAlternateHeaderNames(t *testing.T) {
	uc := &fakePayoutReleaser{}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	req := httptest.NewRequest("POST", "/api/v1/payments/doku/transfer-webhook", strings.NewReader(transferBodyPaid))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+releaseTestToken)
	req.Header.Set("Signature", transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid))
	req.Header.Set("Request-Timestamp", releaseTestTimestamp)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []string{releaseTestRef}, uc.syncedRefs)
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

func TestPayoutReleaseHandler_TransferWebhookUnreadableBodyChangesNothing(t *testing.T) {
	cases := map[string]string{
		"not json":             `{"partnerReferenceNo":`,
		"no partner reference": `{"latestTransactionStatus":"00"}`,
		"empty body":           ``,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			uc := &fakePayoutReleaser{}
			app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

			resp := postTransferWebhook(t, app, body, transferSignature(releaseTestSecret, transferWebhookPath, body))
			defer resp.Body.Close()

			assert.Equal(t, 400, resp.StatusCode)
			assert.Equal(t, "INVALID_NOTIFICATION", errorCode(t, resp))
			assert.Empty(t, uc.syncedRefs, "a body that cannot be read must never reach the usecase")
		})
	}
}

func TestPayoutReleaseHandler_TransferWebhookUnknownTransferIsAcknowledged(t *testing.T) {
	uc := &fakePayoutReleaser{syncErr: apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_NOT_FOUND", "payout not found")}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhook(t, app, transferBodyPaid, transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid))
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(raw), "IGNORED")
}

func TestPayoutReleaseHandler_TransferWebhookStatusApiFailureIsRetryable(t *testing.T) {
	uc := &fakePayoutReleaser{syncErr: errors.New("doku sub-account transactions-status returned HTTP 503")}
	app := newReleaseTestApp(uc, &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhook(t, app, transferBodyPaid, transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid))
	defer resp.Body.Close()

	assert.Equal(t, 500, resp.StatusCode, "a failed status check must be retried by DOKU, not acknowledged")
	assert.Equal(t, "INTERNAL_ERROR", errorCode(t, resp))
}

// The next two tests use the real usecase over an in-memory payout store and a
// fake DOKU gateway. The notification body claims one outcome and DOKU's
// transactions-status API reports the other; the payout must follow the API.

func TestPayoutReleaseHandler_BodyClaimsSuccessButStatusApiSaysFailed(t *testing.T) {
	store, gateway := newSyncFixture(&doku.TransactionStatusResult{LatestStatus: doku.TransactionStatusFailed})
	app := newReleaseTestApp(usecaseSyncer(store, gateway), &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhook(t, app, transferBodyPaid, transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid))
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, []string{releaseTestRef}, gateway.statusQueries)
	stored := store.rows[1]
	assert.Equal(t, entity.PayoutStatusFailed, stored.Status)
	assert.Nil(t, stored.PaidAt)
}

func TestPayoutReleaseHandler_BodyClaimsFailureButStatusApiSaysPaid(t *testing.T) {
	store, gateway := newSyncFixture(&doku.TransactionStatusResult{LatestStatus: doku.TransactionStatusSuccess})
	failedBody := `{"partnerReferenceNo":"payout-booking-42","latestTransactionStatus":"06"}`
	app := newReleaseTestApp(usecaseSyncer(store, gateway), &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)

	resp := postTransferWebhook(t, app, failedBody, transferSignature(releaseTestSecret, transferWebhookPath, failedBody))
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	stored := store.rows[1]
	assert.Equal(t, entity.PayoutStatusPaid, stored.Status)
	assert.NotNil(t, stored.PaidAt)
}

func TestPayoutReleaseHandler_RepeatedNotificationNeverChangesAPaidPayout(t *testing.T) {
	store, gateway := newSyncFixture(&doku.TransactionStatusResult{LatestStatus: doku.TransactionStatusSuccess})
	app := newReleaseTestApp(usecaseSyncer(store, gateway), &fakeProfileUsecase{role: "customer"}, 0, releaseTestSecret, nil)
	signature := transferSignature(releaseTestSecret, transferWebhookPath, transferBodyPaid)

	first := postTransferWebhook(t, app, transferBodyPaid, signature)
	defer first.Body.Close()
	paidAt := store.rows[1].PaidAt
	require.NotNil(t, paidAt)

	// DOKU now answers failed for the same reference. A paid payout must not
	// move, and the status API must not even be asked again.
	gateway.statusResult = &doku.TransactionStatusResult{LatestStatus: doku.TransactionStatusFailed}
	second := postTransferWebhook(t, app, transferBodyPaid, signature)
	defer second.Body.Close()

	assert.Equal(t, 200, second.StatusCode)
	assert.Equal(t, entity.PayoutStatusPaid, store.rows[1].Status)
	assert.Equal(t, paidAt, store.rows[1].PaidAt)
	assert.Len(t, gateway.statusQueries, 1)
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

func newReleaseTestApp(uc payoutReleaser, profiles *fakeProfileUsecase, userID uint, secret string, releaseErr error) *fiber.App {
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
	syncErr           error
	releaseCalls      int
	releasedBookingID uint
	syncedRefs        []string
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

func (f *fakePayoutReleaser) SyncTransferStatus(ctx context.Context, partnerReferenceNo string) (*entity.Payout, error) {
	f.syncedRefs = append(f.syncedRefs, partnerReferenceNo)
	if f.syncErr != nil {
		return nil, f.syncErr
	}
	return &entity.Payout{PartnerReferenceNo: partnerReferenceNo}, nil
}

// syncFakeStore is an in-memory payout repository holding one submitted payout
// (ID 1) under releaseTestRef.
type syncFakeStore struct {
	rows    map[uint]*entity.Payout
	readSeq int64
}

// NextReadExternalID mirrors the Postgres sequence: it starts at the read band.
func (s *syncFakeStore) NextReadExternalID(ctx context.Context) (string, error) {
	if s.readSeq == 0 {
		s.readSeq = repository.PayoutReadExternalIDStart
	}
	id := s.readSeq
	s.readSeq++
	return strconv.FormatInt(id, 10), nil
}

func (s *syncFakeStore) Create(ctx context.Context, p *entity.Payout) error {
	s.rows[p.ID] = p
	return nil
}

func (s *syncFakeStore) FindByID(ctx context.Context, id uint) (*entity.Payout, error) {
	return s.find(func(p *entity.Payout) bool { return p.ID == id })
}

func (s *syncFakeStore) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payout, error) {
	return s.find(func(p *entity.Payout) bool { return p.BookingID == bookingID })
}

func (s *syncFakeStore) FindByPartnerReferenceNo(ctx context.Context, ref string) (*entity.Payout, error) {
	return s.find(func(p *entity.Payout) bool { return p.PartnerReferenceNo == ref })
}

func (s *syncFakeStore) FindSubmitted(ctx context.Context, limit int, reconciledBefore time.Time) ([]*entity.Payout, error) {
	return nil, nil
}

func (s *syncFakeStore) FindStalledInquiry(ctx context.Context, limit int, updatedBefore time.Time) ([]*entity.Payout, error) {
	return nil, nil
}

func (s *syncFakeStore) SumInFlight(ctx context.Context, excludePayoutID uint) (int64, error) {
	var total int64
	for _, row := range s.rows {
		if row.ID != excludePayoutID && (row.Status == entity.PayoutStatusSubmitted || row.Status == entity.PayoutStatusInquiry) {
			total += row.Amount
		}
	}
	return total, nil
}

func (s *syncFakeStore) Transition(ctx context.Context, payoutID uint, from, to string) (bool, error) {
	row, ok := s.rows[payoutID]
	if !ok || row.Status != from {
		return false, nil
	}
	row.Status = to
	return true, nil
}

func (s *syncFakeStore) Update(ctx context.Context, p *entity.Payout) error {
	stored, ok := s.rows[p.ID]
	if !ok {
		return apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_NOT_FOUND", "payout not found")
	}
	copied := *p
	copied.Status = stored.Status
	s.rows[p.ID] = &copied
	return nil
}

func (s *syncFakeStore) find(match func(*entity.Payout) bool) (*entity.Payout, error) {
	for _, row := range s.rows {
		if match(row) {
			copied := *row
			return &copied, nil
		}
	}
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_NOT_FOUND", "payout not found")
}

// syncFakeGateway answers only transactions-status; the other DOKU calls are
// never reached by a sync and fail loudly if they are.
type syncFakeGateway struct {
	statusResult  *doku.TransactionStatusResult
	statusQueries []string
}

func (g *syncFakeGateway) TransferInquiry(ctx context.Context, externalID string, req doku.TransferInquiryRequest) (*doku.TransferInquiryResult, error) {
	return nil, errors.New("unexpected transfer inquiry during a status sync")
}

func (g *syncFakeGateway) TransferPayment(ctx context.Context, externalID string, req doku.TransferPaymentRequest) (*doku.TransferPaymentResult, error) {
	return nil, errors.New("unexpected transfer payment during a status sync")
}

func (g *syncFakeGateway) TransactionStatus(ctx context.Context, externalID string, partnerReferenceNo string) (*doku.TransactionStatusResult, error) {
	g.statusQueries = append(g.statusQueries, partnerReferenceNo)
	return g.statusResult, nil
}

func (g *syncFakeGateway) BalanceInquiry(ctx context.Context, externalID string, profileID string) (*doku.BalanceResult, error) {
	return nil, errors.New("unexpected balance inquiry during a status sync")
}

func newSyncFixture(status *doku.TransactionStatusResult) (*syncFakeStore, *syncFakeGateway) {
	store := &syncFakeStore{rows: map[uint]*entity.Payout{
		1: {ID: 1, BookingID: 42, ProviderID: 7, Amount: 150000, Status: entity.PayoutStatusSubmitted, PartnerReferenceNo: releaseTestRef},
	}}
	return store, &syncFakeGateway{statusResult: status}
}

// usecaseSyncer wires the real payout usecase over the fakes. Only the release
// dependencies the sync path touches are set.
func usecaseSyncer(store *syncFakeStore, gateway *syncFakeGateway) payoutReleaser {
	return usecase.NewPayoutUsecase(nil, nil, nil, "").WithRelease(usecase.PayoutRelease{
		Payouts: store,
		Gateway: gateway,
	})
}

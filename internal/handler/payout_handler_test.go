package handler

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

const payoutRequestBody = `{"bank_code":"014","bank_name":"Bank Central Asia","account_holder_name":"Siti Rahayu","account_number":"1234567890","phone":"6281234567890"}`

func TestPayoutHandler_Upsert_ReturnsMaskedAccount(t *testing.T) {
	uc := &fakePayoutUsecase{account: &entity.PayoutAccount{
		UserID: 71, BankCode: "014", BankName: "Bank Central Asia",
		AccountNumberEncrypted: "enc-secret", AccountHolderNameEncrypted: "enc-holder",
		AccountLast4: "7890", PhoneLast4: "7890", Status: entity.PayoutAccountStatusUnverified,
	}}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 71)

	req := httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(payoutRequestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, body, "7890")
	assert.NotContains(t, body, "1234567890")
	assert.NotContains(t, body, "enc-secret")
	assert.NotContains(t, body, "Siti Rahayu")
}

func TestPayoutHandler_Upsert_RejectsNonProvider(t *testing.T) {
	uc := &fakePayoutUsecase{}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "customer"}, 71)

	req := httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(payoutRequestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode)
	assert.False(t, uc.upsertCalled, "a customer must not be able to create a payout destination")
}

func TestPayoutHandler_Upsert_StructValidationIs400AndSkipsUsecase(t *testing.T) {
	cases := map[string]string{
		"missing bank_code": `{"bank_name":"Bank Central Asia","account_holder_name":"Siti Rahayu","account_number":"1234567890","phone":"6281234567890"}`,
		"empty object":      `{}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			uc := &fakePayoutUsecase{}
			app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 71)

			req := httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, 400, resp.StatusCode)
			assert.Equal(t, "VALIDATION_ERROR", errorCode(t, resp))
			assert.False(t, uc.upsertCalled, "an invalid body must never reach the usecase")
		})
	}
}

// A zero-byte body fails at bind, before validation, so it is a 400 with
// INVALID_REQUEST rather than VALIDATION_ERROR. It still must not reach the usecase.
func TestPayoutHandler_Upsert_EmptyBodyIs400AndSkipsUsecase(t *testing.T) {
	uc := &fakePayoutUsecase{}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 71)

	req := httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
	assert.False(t, uc.upsertCalled, "an empty body must never reach the usecase")
}

func TestPayoutHandler_Upsert_InvalidValueIs422(t *testing.T) {
	invalidBody := `{"bank_code":"bca","bank_name":"Bank Central Asia","account_holder_name":"Siti Rahayu","account_number":"1234567890","phone":"6281234567890"}`
	uc := &fakePayoutUsecase{err: apperror.NewDomainError(apperror.ErrValidation, "INVALID_PAYOUT_ACCOUNT", "bank_code must be a 2 to 8 digit BI bank code")}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 71)

	req := httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(invalidBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 422, resp.StatusCode)
	assert.Equal(t, "INVALID_PAYOUT_ACCOUNT", errorCode(t, resp))
	assert.True(t, uc.upsertCalled)
}

func TestPayoutHandler_Upsert_MalformedJSONIs400(t *testing.T) {
	uc := &fakePayoutUsecase{}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 71)

	req := httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(`{"bank_code":`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)
	assert.Equal(t, "INVALID_REQUEST", errorCode(t, resp))
	assert.False(t, uc.upsertCalled, "a malformed body must never reach the usecase")
}

func TestPayoutHandler_Upsert_DuplicateAccountIs409(t *testing.T) {
	uc := &fakePayoutUsecase{err: apperror.NewDomainError(apperror.ErrAlreadyExists, "PAYOUT_ACCOUNT_DUPLICATE", "this bank account is already used by another mitra")}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 71)

	req := httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(payoutRequestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 409, resp.StatusCode)
	assert.Equal(t, "PAYOUT_ACCOUNT_DUPLICATE", errorCode(t, resp))
}

func TestPayoutHandler_Upsert_RequiresSession(t *testing.T) {
	uc := &fakePayoutUsecase{}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 0)

	req := httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(payoutRequestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
	assert.False(t, uc.upsertCalled)
}

func TestPayoutHandler_Get_RejectsNonProvider(t *testing.T) {
	uc := &fakePayoutUsecase{account: &entity.PayoutAccount{UserID: 71, AccountLast4: "7890"}}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "customer"}, 71)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/payout-account", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode)
	assert.False(t, uc.getCalled, "a customer must not be able to read a payout destination")
}

func TestPayoutHandler_Get_ReturnsMaskedAccount(t *testing.T) {
	uc := &fakePayoutUsecase{account: &entity.PayoutAccount{
		UserID: 71, BankCode: "014", BankName: "Bank Central Asia",
		AccountNumberEncrypted: "enc-secret", AccountHolderNameEncrypted: "enc-holder",
		AccountLast4: "7890", PhoneLast4: "7890", Status: entity.PayoutAccountStatusUnverified,
	}}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 71)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/payout-account", nil))
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, body, "7890")
	assert.NotContains(t, body, "enc-secret")
	assert.NotContains(t, body, "enc-holder")
}

func TestPayoutHandler_Get_NotFoundIs404(t *testing.T) {
	uc := &fakePayoutUsecase{err: apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_ACCOUNT_NOT_FOUND", "no payout account")}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 71)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/payout-account", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "PAYOUT_ACCOUNT_NOT_FOUND", errorCode(t, resp))
}

func TestPayoutHandler_RequiresSession(t *testing.T) {
	uc := &fakePayoutUsecase{}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 0)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/payout-account", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
	assert.False(t, uc.getCalled)
}

func TestPayoutHandler_ProfileLookupFailureIs401(t *testing.T) {
	uc := &fakePayoutUsecase{}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{err: errors.New("db down")}, 71)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/payout-account", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
	assert.False(t, uc.getCalled)
}

func TestPayoutHandler_MissingProfileIs401(t *testing.T) {
	uc := &fakePayoutUsecase{}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{nilProfile: true}, 71)

	resp, err := app.Test(httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(payoutRequestBody)))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
	assert.False(t, uc.upsertCalled)
}

func newPayoutTestApp(uc *fakePayoutUsecase, profiles *fakeProfileUsecase, userID uint) *fiber.App {
	h := NewPayoutHandler(uc, profiles)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, userID)
		return c.Next()
	})
	app.Put("/api/v1/payout-account", h.Upsert)
	app.Get("/api/v1/payout-account", h.Get)
	return app
}

func errorCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &body), "body: %s", raw)
	return body.Code
}

type fakePayoutUsecase struct {
	account      *entity.PayoutAccount
	err          error
	upsertCalled bool
	getCalled    bool
}

func (f *fakePayoutUsecase) Upsert(ctx context.Context, userID uint, req dto.PayoutAccountRequest) (*entity.PayoutAccount, error) {
	f.upsertCalled = true
	return f.result()
}

func (f *fakePayoutUsecase) Get(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	f.getCalled = true
	return f.result()
}

// result returns an empty account when none is configured, so a test that
// reaches the usecase by mistake fails on its assertions, not on a nil panic.
func (f *fakePayoutUsecase) result() (*entity.PayoutAccount, error) {
	if f.account == nil && f.err == nil {
		return &entity.PayoutAccount{}, nil
	}
	return f.account, f.err
}

type fakeProfileUsecase struct {
	role       string
	err        error
	nilProfile bool
}

func (f *fakeProfileUsecase) GetProfile(ctx context.Context, userID uint) (*entity.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.nilProfile {
		return nil, nil
	}
	return &entity.User{ID: userID, Role: f.role}, nil
}

package handler

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

const payoutRequestBody = `{"bank_code":"BCAIDJA","bank_name":"Bank Central Asia","account_holder_name":"Siti Rahayu","account_number":"1234567890"}`

func TestPayoutHandler_Upsert_ReturnsMaskedAccount(t *testing.T) {
	uc := &fakePayoutUsecase{account: &entity.PayoutAccount{
		UserID: 71, BankCode: "BCAIDJA", BankName: "Bank Central Asia",
		AccountNumberEncrypted: "enc-secret", AccountHolderNameEncrypted: "enc-holder",
		AccountLast4: "7890", Status: entity.PayoutAccountStatusUnverified,
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
	assert.False(t, uc.decryptCalled, "the handler must never decrypt the destination")
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

func TestPayoutHandler_Get_RejectsNonProvider(t *testing.T) {
	uc := &fakePayoutUsecase{account: &entity.PayoutAccount{UserID: 71, AccountLast4: "7890"}}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "customer"}, 71)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/payout-account", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode)
}

func TestPayoutHandler_Get_ReturnsMaskedAccount(t *testing.T) {
	uc := &fakePayoutUsecase{account: &entity.PayoutAccount{
		UserID: 71, BankCode: "BCAIDJA", BankName: "Bank Central Asia",
		AccountNumberEncrypted: "enc-secret", AccountHolderNameEncrypted: "enc-holder",
		AccountLast4: "7890", Status: entity.PayoutAccountStatusUnverified,
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
}

func TestPayoutHandler_RequiresSession(t *testing.T) {
	uc := &fakePayoutUsecase{}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{role: "provider"}, 0)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/payout-account", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
}

func TestPayoutHandler_ProfileLookupFailureIs401(t *testing.T) {
	uc := &fakePayoutUsecase{}
	app := newPayoutTestApp(uc, &fakeProfileUsecase{err: errors.New("db down")}, 71)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/payout-account", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
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

type fakePayoutUsecase struct {
	account       *entity.PayoutAccount
	err           error
	upsertCalled  bool
	decryptCalled bool
}

func (f *fakePayoutUsecase) Upsert(ctx context.Context, userID uint, req dto.PayoutAccountRequest) (*entity.PayoutAccount, error) {
	f.upsertCalled = true
	return f.account, f.err
}

func (f *fakePayoutUsecase) Get(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	return f.account, f.err
}

func (f *fakePayoutUsecase) DecryptDestination(ctx context.Context, userID uint) (usecase.PayoutDestination, error) {
	f.decryptCalled = true
	return usecase.PayoutDestination{}, nil
}

type fakeProfileUsecase struct {
	role string
	err  error
}

func (f *fakeProfileUsecase) GetProfile(ctx context.Context, userID uint) (*entity.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &entity.User{ID: userID, Role: f.role}, nil
}

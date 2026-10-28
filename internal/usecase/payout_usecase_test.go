package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

type fakeSecretBox struct{}

func (f *fakeSecretBox) Encrypt(plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (f *fakeSecretBox) Decrypt(ciphertext string) (string, error) {
	plaintext, ok := strings.CutPrefix(ciphertext, "enc:")
	if !ok {
		return "", errors.New("bad ciphertext")
	}
	return plaintext, nil
}

func (f *fakeSecretBox) Fingerprint(plaintext string) string { return "fp:" + plaintext }

type fakePayoutAccountRepo struct {
	byUser         map[uint]*entity.PayoutAccount
	fingerprintErr error
}

func (f *fakePayoutAccountRepo) Upsert(ctx context.Context, a *entity.PayoutAccount) error {
	if f.byUser == nil {
		f.byUser = map[uint]*entity.PayoutAccount{}
	}
	a.ID = 1
	f.byUser[a.UserID] = a
	return nil
}

func (f *fakePayoutAccountRepo) FindByUserID(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	a, ok := f.byUser[userID]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_ACCOUNT_NOT_FOUND", "payout account not found")
	}
	return a, nil
}

func (f *fakePayoutAccountRepo) FindByFingerprint(ctx context.Context, fingerprint string) (*entity.PayoutAccount, error) {
	if f.fingerprintErr != nil {
		return nil, f.fingerprintErr
	}
	for _, a := range f.byUser {
		if a.AccountFingerprint == fingerprint {
			return a, nil
		}
	}
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_ACCOUNT_NOT_FOUND", "payout account not found")
}

type fakeUserNameLookup struct {
	names map[uint]string
}

func (f *fakeUserNameLookup) FindByID(ctx context.Context, id uint) (*entity.User, error) {
	name, ok := f.names[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
	}
	return &entity.User{ID: id, Name: name}, nil
}

func newTestPayoutUsecase(repo *fakePayoutAccountRepo) *PayoutUsecase {
	users := &fakeUserNameLookup{names: map[uint]string{71: "Siti Rahayu", 72: "Rudi Hartono"}}
	return NewPayoutUsecase(repo, users, &fakeSecretBox{}, "v1")
}

func TestPayoutUsecase_Upsert_EncryptsAndNeverReturnsPlaintext(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode:          "CENAIDJA",
		BankName:          "Bank Central Asia",
		AccountHolderName: "Siti Rahayu",
		AccountNumber:     "1234567890",
	})
	require.NoError(t, err)

	assert.Equal(t, "enc:1234567890", account.AccountNumberEncrypted)
	assert.Equal(t, "enc:Siti Rahayu", account.AccountHolderNameEncrypted)
	assert.Equal(t, "7890", account.AccountLast4)
	assert.Equal(t, "fp:1234567890", account.AccountFingerprint)
	assert.Equal(t, "v1", account.EncryptionKeyID)
	assert.Equal(t, entity.PayoutAccountStatusUnverified, account.Status)
	assert.False(t, account.NameMismatch)
}

func TestPayoutUsecase_Upsert_RejectsAnAccountAlreadyUsedByAnotherMitra(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)

	_, err = uc.Upsert(context.Background(), 72, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "Rudi Hartono", AccountNumber: "1234567890",
	})
	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "PAYOUT_ACCOUNT_DUPLICATE", domainErr.Code)
}

func TestPayoutUsecase_Upsert_AllowsTheSameUserToResaveTheirAccount(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank Central Asia", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)
	assert.Equal(t, "Bank Central Asia", account.BankName)
}

func TestPayoutUsecase_Upsert_PropagatesFingerprintLookupErrors(t *testing.T) {
	dbErr := errors.New("connection reset")
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{fingerprintErr: dbErr})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	assert.ErrorIs(t, err, dbErr)
}

func TestPayoutUsecase_Upsert_RejectsInvalidBankCodeAndAccountNumber(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BC", BankName: "Bank", AccountHolderName: "Siti", AccountNumber: "1234567890",
	})
	assert.Error(t, err)

	_, err = uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "Siti", AccountNumber: "12-34",
	})
	assert.Error(t, err)
}

func TestPayoutUsecase_Upsert_ValidationErrorsNeverEchoTheAccountNumber(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "12-34",
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "12-34")
}

func TestPayoutUsecase_Upsert_FlagsNameMismatchWithoutFailing(t *testing.T) {
	repo := &fakePayoutAccountRepo{}
	uc := newTestPayoutUsecase(repo)

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "PT Contoh", AccountNumber: "1234567890",
	})
	require.NoError(t, err)
	assert.True(t, account.NameMismatch)
	assert.Equal(t, entity.PayoutAccountStatusUnverified, account.Status)
	assert.Equal(t, account, repo.byUser[71])
}

func TestPayoutUsecase_Upsert_MatchesNameIgnoringCaseSpacesAndPunctuation(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "SITI-RAHAYU.", AccountNumber: "1234567890",
	})
	require.NoError(t, err)
	assert.False(t, account.NameMismatch)
}

func TestPayoutUsecase_Upsert_FailsWhenTheUserIsUnknown(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 99, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	assert.Error(t, err)
}

func TestPayoutUsecase_DecryptDestination_ReturnsPlaintextOnlyHere(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)

	destination, err := uc.DecryptDestination(context.Background(), 71)
	require.NoError(t, err)
	assert.Equal(t, "1234567890", destination.AccountNumber)
	assert.Equal(t, "Siti Rahayu", destination.AccountHolderName)
	assert.Equal(t, "CENAIDJA", destination.BankCode)
}

func TestPayoutUsecase_Get_ReturnsStoredAccountWithoutDecrypting(t *testing.T) {
	uc := newTestPayoutUsecase(&fakePayoutAccountRepo{})

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "CENAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)

	account, err := uc.Get(context.Background(), 71)
	require.NoError(t, err)
	assert.Equal(t, "7890", account.AccountLast4)
	assert.Equal(t, "enc:1234567890", account.AccountNumberEncrypted)
}

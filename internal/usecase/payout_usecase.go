package usecase

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

// SecretBox is the encryption boundary the payout usecase depends on.
type SecretBox interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
	Fingerprint(plaintext string) string
}

// UserNameLookup is the part of the user repository the payout usecase needs
// to compare the submitted holder name with the registered name.
type UserNameLookup interface {
	FindByID(ctx context.Context, id uint) (*entity.User, error)
}

// PayoutDestination is the decrypted form, used only when paying out.
type PayoutDestination struct {
	BankCode          string
	BankName          string
	AccountHolderName string
	AccountNumber     string
}

type PayoutUsecase struct {
	repo            repository.PayoutAccountRepository
	users           UserNameLookup
	secret          SecretBox
	encryptionKeyID string
}

func NewPayoutUsecase(repo repository.PayoutAccountRepository, users UserNameLookup, secret SecretBox, encryptionKeyID string) *PayoutUsecase {
	if encryptionKeyID == "" {
		encryptionKeyID = "v1"
	}
	return &PayoutUsecase{repo: repo, users: users, secret: secret, encryptionKeyID: encryptionKeyID}
}

func (uc *PayoutUsecase) Upsert(ctx context.Context, userID uint, req dto.PayoutAccountRequest) (*entity.PayoutAccount, error) {
	if err := req.Validate(); err != nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_PAYOUT_ACCOUNT", err.Error())
	}

	fingerprint := uc.secret.Fingerprint(req.AccountNumber)
	// Two Mitras must not share one bank account: the payout would go to
	// one of them. The fingerprint detects this without decrypting anything.
	existing, err := uc.repo.FindByFingerprint(ctx, fingerprint)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		return nil, err
	}
	if err == nil && existing != nil && existing.UserID != userID {
		return nil, apperror.NewDomainError(apperror.ErrAlreadyExists, "PAYOUT_ACCOUNT_DUPLICATE", "this bank account is already used by another mitra")
	}

	user, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	holderName := strings.TrimSpace(req.AccountHolderName)
	encryptedNumber, err := uc.secret.Encrypt(req.AccountNumber)
	if err != nil {
		return nil, err
	}
	encryptedHolder, err := uc.secret.Encrypt(holderName)
	if err != nil {
		return nil, err
	}

	// A re-save always starts unverified: a changed account number must be
	// validated again.
	account := &entity.PayoutAccount{
		UserID:                     userID,
		BankCode:                   req.BankCode,
		BankName:                   strings.TrimSpace(req.BankName),
		AccountHolderNameEncrypted: encryptedHolder,
		AccountNumberEncrypted:     encryptedNumber,
		AccountLast4:               lastFour(req.AccountNumber),
		AccountFingerprint:         fingerprint,
		EncryptionKeyID:            uc.encryptionKeyID,
		Status:                     entity.PayoutAccountStatusUnverified,
		// Flagged, not blocked: the production entity (PT/CV) is unresolved.
		NameMismatch: !sameName(holderName, user.Name),
	}
	if err := uc.repo.Upsert(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

// Get returns the stored account (still encrypted) so the handler can mask it.
func (uc *PayoutUsecase) Get(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	return uc.repo.FindByUserID(ctx, userID)
}

// DecryptDestination returns the plain destination for a payout. Only the
// future transfer path may call it; it never reaches a handler.
func (uc *PayoutUsecase) DecryptDestination(ctx context.Context, userID uint) (PayoutDestination, error) {
	account, err := uc.repo.FindByUserID(ctx, userID)
	if err != nil {
		return PayoutDestination{}, err
	}
	number, err := uc.secret.Decrypt(account.AccountNumberEncrypted)
	if err != nil {
		return PayoutDestination{}, err
	}
	holder, err := uc.secret.Decrypt(account.AccountHolderNameEncrypted)
	if err != nil {
		return PayoutDestination{}, err
	}
	return PayoutDestination{
		BankCode:          account.BankCode,
		BankName:          account.BankName,
		AccountHolderName: holder,
		AccountNumber:     number,
	}, nil
}

// sameName compares two names case-insensitively, ignoring spaces and
// punctuation, so "Siti Rahayu" matches "SITI RAHAYU" but "PT Contoh" does not
// match "Contoh".
func sameName(a, b string) bool {
	return normalizeName(a) == normalizeName(b)
}

func normalizeName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// lastFour returns the last four characters of an account number for masked
// display. The full value is never stored in plaintext.
func lastFour(accountNumber string) string {
	if len(accountNumber) <= 4 {
		return accountNumber
	}
	return accountNumber[len(accountNumber)-4:]
}

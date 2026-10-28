package gormrepo

import (
	"context"
	"errors"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PayoutAccountRepository struct {
	db *gorm.DB
}

func NewPayoutAccountRepository(db *gorm.DB) *PayoutAccountRepository {
	return &PayoutAccountRepository{db: db}
}

func (r *PayoutAccountRepository) Upsert(ctx context.Context, account *entity.PayoutAccount) error {
	m := toPayoutAccountModel(account)
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"bank_code", "bank_name", "account_holder_name_encrypted",
			"account_number_encrypted", "account_last4", "account_fingerprint",
			"encryption_key_id", "status", "name_mismatch", "validated_at",
			"last_error", "updated_at",
		}),
	}, clause.Returning{}).Create(m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return payoutAccountDuplicateError()
		}
		return payoutAccountDBError(err)
	}
	*account = *toPayoutAccountEntity(m)
	return nil
}

func (r *PayoutAccountRepository) FindByUserID(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	var m model.PayoutAccountModel
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, payoutAccountNotFoundError()
		}
		return nil, payoutAccountDBError(err)
	}
	return toPayoutAccountEntity(&m), nil
}

func (r *PayoutAccountRepository) FindByFingerprint(ctx context.Context, fingerprint string) (*entity.PayoutAccount, error) {
	var m model.PayoutAccountModel
	err := r.db.WithContext(ctx).Where("account_fingerprint = ?", fingerprint).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, payoutAccountNotFoundError()
		}
		return nil, payoutAccountDBError(err)
	}
	return toPayoutAccountEntity(&m), nil
}

func toPayoutAccountModel(a *entity.PayoutAccount) *model.PayoutAccountModel {
	return &model.PayoutAccountModel{
		ID:                         a.ID,
		UserID:                     a.UserID,
		BankCode:                   a.BankCode,
		BankName:                   a.BankName,
		AccountHolderNameEncrypted: a.AccountHolderNameEncrypted,
		AccountNumberEncrypted:     a.AccountNumberEncrypted,
		AccountLast4:               a.AccountLast4,
		AccountFingerprint:         a.AccountFingerprint,
		EncryptionKeyID:            a.EncryptionKeyID,
		Status:                     a.Status,
		NameMismatch:               a.NameMismatch,
		ValidatedAt:                a.ValidatedAt,
		LastError:                  a.LastError,
		CreatedAt:                  a.CreatedAt,
		UpdatedAt:                  a.UpdatedAt,
	}
}

func toPayoutAccountEntity(m *model.PayoutAccountModel) *entity.PayoutAccount {
	return &entity.PayoutAccount{
		ID:                         m.ID,
		UserID:                     m.UserID,
		BankCode:                   m.BankCode,
		BankName:                   m.BankName,
		AccountHolderNameEncrypted: m.AccountHolderNameEncrypted,
		AccountNumberEncrypted:     m.AccountNumberEncrypted,
		AccountLast4:               m.AccountLast4,
		AccountFingerprint:         m.AccountFingerprint,
		EncryptionKeyID:            m.EncryptionKeyID,
		Status:                     m.Status,
		NameMismatch:               m.NameMismatch,
		ValidatedAt:                m.ValidatedAt,
		LastError:                  m.LastError,
		CreatedAt:                  m.CreatedAt,
		UpdatedAt:                  m.UpdatedAt,
	}
}

func payoutAccountNotFoundError() error {
	return apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_ACCOUNT_NOT_FOUND", "payout account not found")
}

// payoutAccountDuplicateError reports that the fingerprint is already stored
// for another user. The unique index on account_fingerprint is the guard that
// holds even when two saves race past the usecase's pre-check.
func payoutAccountDuplicateError() error {
	return apperror.NewDomainError(apperror.ErrAlreadyExists, "PAYOUT_ACCOUNT_DUPLICATE", "this bank account is already used by another mitra")
}

func payoutAccountDBError(err error) error {
	return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
}

package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

type PayoutRepository struct {
	db *gorm.DB
}

func NewPayoutRepository(db *gorm.DB) *PayoutRepository {
	return &PayoutRepository{db: db}
}

func (r *PayoutRepository) Create(ctx context.Context, payout *entity.Payout) error {
	m := toPayoutModel(payout)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return payoutDuplicateError()
		}
		return payoutDBError(err)
	}

	payout.ID = m.ID
	payout.CreatedAt = m.CreatedAt
	payout.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *PayoutRepository) FindByID(ctx context.Context, id uint) (*entity.Payout, error) {
	return r.findOne(ctx, "id = ?", id)
}

func (r *PayoutRepository) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payout, error) {
	return r.findOne(ctx, "booking_id = ?", bookingID)
}

func (r *PayoutRepository) FindByPartnerReferenceNo(ctx context.Context, partnerReferenceNo string) (*entity.Payout, error) {
	return r.findOne(ctx, "partner_reference_no = ?", partnerReferenceNo)
}

// FindSubmitted returns up to limit submitted payouts that were not reconciled
// since reconciledBefore, never-reconciled rows first, so a payout DOKU keeps
// answering about cannot starve the rest.
func (r *PayoutRepository) FindSubmitted(ctx context.Context, limit int, reconciledBefore time.Time) ([]*entity.Payout, error) {
	return r.findBatch(ctx, limit,
		r.db.WithContext(ctx).
			Where("status = ?", entity.PayoutStatusSubmitted).
			Where("last_reconciled_at IS NULL OR last_reconciled_at < ?", reconciledBefore).
			Order("last_reconciled_at ASC NULLS FIRST, updated_at ASC, id ASC"))
}

// FindStalledInquiry returns up to limit payouts stuck in inquiry since before
// updatedBefore, oldest first. A payout moves to inquiry before any money is
// sent, so these are the candidates for a re-drive.
func (r *PayoutRepository) FindStalledInquiry(ctx context.Context, limit int, updatedBefore time.Time) ([]*entity.Payout, error) {
	return r.findBatch(ctx, limit,
		r.db.WithContext(ctx).
			Where("status = ?", entity.PayoutStatusInquiry).
			Where("updated_at < ?", updatedBefore).
			Order("updated_at ASC, id ASC"))
}

func (r *PayoutRepository) findBatch(ctx context.Context, limit int, query *gorm.DB) ([]*entity.Payout, error) {
	if limit <= 0 {
		return []*entity.Payout{}, nil
	}
	var models []model.PayoutModel
	if err := query.Limit(limit).Find(&models).Error; err != nil {
		return nil, payoutDBError(err)
	}
	out := make([]*entity.Payout, 0, len(models))
	for i := range models {
		out = append(out, toPayoutEntity(&models[i]))
	}
	return out, nil
}

// SumInFlight totals the payouts that may already hold money at DOKU: those
// submitted, and those still in inquiry, whose payment may have gone out.
func (r *PayoutRepository) SumInFlight(ctx context.Context, excludePayoutID uint) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).
		Model(&model.PayoutModel{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("status IN ?", []string{entity.PayoutStatusSubmitted, entity.PayoutStatusInquiry}).
		Where("id <> ?", excludePayoutID).
		Scan(&total).Error
	if err != nil {
		return 0, payoutDBError(err)
	}
	return total, nil
}

// Transition is a compare-and-set on status: the row moves only if it is still
// in from. A zero RowsAffected means another writer got there first.
func (r *PayoutRepository) Transition(ctx context.Context, payoutID uint, from, to string) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&model.PayoutModel{}).
		Where("id = ? AND status = ?", payoutID, from).
		Updates(map[string]interface{}{
			"status":     to,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return false, payoutDBError(result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (r *PayoutRepository) Update(ctx context.Context, payout *entity.Payout) error {
	result := r.db.WithContext(ctx).
		Model(&model.PayoutModel{}).
		Where("id = ?", payout.ID).
		Updates(payoutUpdates(payout))
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return payoutDuplicateError()
		}
		return payoutDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return payoutNotFoundError()
	}
	return nil
}

func (r *PayoutRepository) findOne(ctx context.Context, query string, arg interface{}) (*entity.Payout, error) {
	var m model.PayoutModel
	err := r.db.WithContext(ctx).Where(query, arg).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, payoutNotFoundError()
		}
		return nil, payoutDBError(err)
	}
	return toPayoutEntity(&m), nil
}

// payoutUpdates lists every mutable column except status, which only
// Transition writes. The booking, the creation time and the primary key are
// fixed once the row exists. The nullable keys map "" to NULL.
func payoutUpdates(p *entity.Payout) map[string]interface{} {
	return map[string]interface{}{
		"provider_id":               p.ProviderID,
		"amount":                    p.Amount,
		"currency":                  p.Currency,
		"channel":                   p.Channel,
		"partner_reference_no":      nullableString(p.PartnerReferenceNo),
		"idempotency_key":           nullableString(p.IdempotencyKey),
		"request_fingerprint":       p.RequestFingerprint,
		"external_id":               nullableString(p.ExternalID),
		"doku_inquiry_reference_no": p.DokuInquiryReferenceNo,
		"doku_reference_no":         p.DokuReferenceNo,
		"doku_reference_number":     p.DokuReferenceNumber,
		"beneficiary_bank_code":     p.BeneficiaryBankCode,
		"beneficiary_account_holder_name_encrypted": p.BeneficiaryAccountHolderNameEncrypted,
		"beneficiary_account_number_encrypted":      p.BeneficiaryAccountNumberEncrypted,
		"beneficiary_last4":                         p.BeneficiaryLast4,
		"beneficiary_fingerprint":                   p.BeneficiaryFingerprint,
		"replayed":                                  p.Replayed,
		"last_error":                                p.LastError,
		"paid_at":                                   p.PaidAt,
		"last_reconciled_at":                        p.LastReconciledAt,
		"read_attempts":                             p.ReadAttempts,
		"updated_at":                                time.Now(),
	}
}

func toPayoutModel(p *entity.Payout) *model.PayoutModel {
	return &model.PayoutModel{
		ID:                                    p.ID,
		BookingID:                             p.BookingID,
		ProviderID:                            p.ProviderID,
		Amount:                                p.Amount,
		Currency:                              p.Currency,
		Channel:                               p.Channel,
		Status:                                p.Status,
		PartnerReferenceNo:                    nullableString(p.PartnerReferenceNo),
		IdempotencyKey:                        nullableString(p.IdempotencyKey),
		RequestFingerprint:                    p.RequestFingerprint,
		ExternalID:                            nullableString(p.ExternalID),
		DokuInquiryReferenceNo:                p.DokuInquiryReferenceNo,
		DokuReferenceNo:                       p.DokuReferenceNo,
		DokuReferenceNumber:                   p.DokuReferenceNumber,
		BeneficiaryBankCode:                   p.BeneficiaryBankCode,
		BeneficiaryAccountHolderNameEncrypted: p.BeneficiaryAccountHolderNameEncrypted,
		BeneficiaryAccountNumberEncrypted:     p.BeneficiaryAccountNumberEncrypted,
		BeneficiaryLast4:                      p.BeneficiaryLast4,
		BeneficiaryFingerprint:                p.BeneficiaryFingerprint,
		Replayed:                              p.Replayed,
		LastError:                             p.LastError,
		PaidAt:                                p.PaidAt,
		LastReconciledAt:                      p.LastReconciledAt,
		ReadAttempts:                          p.ReadAttempts,
		CreatedAt:                             p.CreatedAt,
		UpdatedAt:                             p.UpdatedAt,
	}
}

func toPayoutEntity(m *model.PayoutModel) *entity.Payout {
	return &entity.Payout{
		ID:                                    m.ID,
		BookingID:                             m.BookingID,
		ProviderID:                            m.ProviderID,
		Amount:                                m.Amount,
		Currency:                              m.Currency,
		Channel:                               m.Channel,
		Status:                                m.Status,
		PartnerReferenceNo:                    stringValue(m.PartnerReferenceNo),
		IdempotencyKey:                        stringValue(m.IdempotencyKey),
		RequestFingerprint:                    m.RequestFingerprint,
		ExternalID:                            stringValue(m.ExternalID),
		DokuInquiryReferenceNo:                m.DokuInquiryReferenceNo,
		DokuReferenceNo:                       m.DokuReferenceNo,
		DokuReferenceNumber:                   m.DokuReferenceNumber,
		BeneficiaryBankCode:                   m.BeneficiaryBankCode,
		BeneficiaryAccountHolderNameEncrypted: m.BeneficiaryAccountHolderNameEncrypted,
		BeneficiaryAccountNumberEncrypted:     m.BeneficiaryAccountNumberEncrypted,
		BeneficiaryLast4:                      m.BeneficiaryLast4,
		BeneficiaryFingerprint:                m.BeneficiaryFingerprint,
		Replayed:                              m.Replayed,
		LastError:                             m.LastError,
		PaidAt:                                m.PaidAt,
		LastReconciledAt:                      m.LastReconciledAt,
		ReadAttempts:                          m.ReadAttempts,
		CreatedAt:                             m.CreatedAt,
		UpdatedAt:                             m.UpdatedAt,
	}
}

func payoutNotFoundError() error {
	return apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_NOT_FOUND", "payout not found")
}

// payoutDuplicateError reports that the booking already has a payout, or that
// the partner reference number is in use. The unique indexes are the guard that
// holds even when two writes race past the usecase's pre-check.
func payoutDuplicateError() error {
	return apperror.NewDomainError(apperror.ErrAlreadyExists, "PAYOUT_DUPLICATE", "a payout already exists for this booking or reference")
}

func payoutDBError(err error) error {
	return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
}

var _ repository.PayoutRepository = (*PayoutRepository)(nil)

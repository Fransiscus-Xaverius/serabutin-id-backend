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
	"gorm.io/gorm/clause"
)

type PaymentRepository struct {
	db *gorm.DB
}

func NewPaymentRepository(db *gorm.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) Create(ctx context.Context, payment *entity.Payment) error {
	m := toPaymentModel(payment)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return paymentDBError(err)
	}

	payment.ID = m.ID
	payment.CreatedAt = m.CreatedAt
	payment.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *PaymentRepository) FindByBookingID(ctx context.Context, bookingID uint) (*entity.Payment, error) {
	var m model.PaymentModel
	err := r.db.WithContext(ctx).Where("booking_id = ?", bookingID).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, paymentNotFoundError()
		}
		return nil, paymentDBError(err)
	}
	return toPaymentEntity(&m), nil
}

func (r *PaymentRepository) FindByInvoiceNumber(ctx context.Context, invoiceNumber string) (*entity.Payment, error) {
	var m model.PaymentModel
	err := r.db.WithContext(ctx).Where("invoice_number = ?", invoiceNumber).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, paymentNotFoundError()
		}
		return nil, paymentDBError(err)
	}
	return toPaymentEntity(&m), nil
}

// FindByIdempotencyKey returns the payment that carries the key. The key is
// derived from the row itself, so a retry of that row finds it here and can
// continue with the same key instead of creating a second attempt.
func (r *PaymentRepository) FindByIdempotencyKey(ctx context.Context, key string) (*entity.Payment, error) {
	var m model.PaymentModel
	err := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, paymentNotFoundError()
		}
		return nil, paymentDBError(err)
	}
	return toPaymentEntity(&m), nil
}

// UpdateIdempotency stores the key and fingerprint of an attempt. The key
// is nullable, so an empty key is stored as NULL.
func (r *PaymentRepository) UpdateIdempotency(ctx context.Context, paymentID uint, key, fingerprint string) error {
	updates := map[string]interface{}{
		"idempotency_key":     nullableString(key),
		"request_fingerprint": fingerprint,
	}
	result := r.db.WithContext(ctx).Model(&model.PaymentModel{}).Where("id = ?", paymentID).Updates(updates)
	if result.Error != nil {
		return paymentDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return paymentNotFoundError()
	}
	return nil
}

func (r *PaymentRepository) MarkReplayed(ctx context.Context, paymentID uint, replayed bool) error {
	result := r.db.WithContext(ctx).
		Model(&model.PaymentModel{}).
		Where("id = ?", paymentID).
		Update("replayed", replayed)
	if result.Error != nil {
		return paymentDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return paymentNotFoundError()
	}
	return nil
}

func (r *PaymentRepository) UpdateCheckout(ctx context.Context, id uint, checkout repository.PaymentCheckout) error {
	updates := map[string]interface{}{
		"payment_url":         checkout.PaymentURL,
		"provider_request_id": checkout.ProviderRequestID,
		"provider_session_id": checkout.ProviderSessionID,
		"expires_at":          checkout.ExpiresAt,
	}
	result := r.db.WithContext(ctx).Model(&model.PaymentModel{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return paymentDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return paymentNotFoundError()
	}
	return nil
}

func (r *PaymentRepository) UpdateStatus(ctx context.Context, id uint, status string, paidAt *time.Time) error {
	updates := map[string]interface{}{"status": status}
	if paidAt != nil {
		updates["paid_at"] = paidAt
	}
	result := r.db.WithContext(ctx).Model(&model.PaymentModel{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return paymentDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return paymentNotFoundError()
	}
	return nil
}

func (r *PaymentRepository) UpdateStatusIfPending(ctx context.Context, id uint, status string) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&model.PaymentModel{}).
		Where("id = ? AND status = ?", id, entity.PaymentStatusPending).
		Update("status", status)
	if result.Error != nil {
		return false, paymentDBError(result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *PaymentRepository) UpdateLastError(ctx context.Context, id uint, lastError string) error {
	result := r.db.WithContext(ctx).
		Model(&model.PaymentModel{}).
		Where("id = ?", id).
		Update("last_error", lastError)
	if result.Error != nil {
		return paymentDBError(result.Error)
	}
	if result.RowsAffected == 0 {
		return paymentNotFoundError()
	}
	return nil
}

func (r *PaymentRepository) FindPendingExpired(ctx context.Context, now time.Time) ([]*entity.Payment, error) {
	var models []model.PaymentModel
	err := r.db.WithContext(ctx).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at < ?", entity.PaymentStatusPending, now).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, paymentDBError(err)
	}

	payments := make([]*entity.Payment, len(models))
	for i := range models {
		payments[i] = toPaymentEntity(&models[i])
	}
	return payments, nil
}

func (r *PaymentRepository) MarkPaidAndActivateBooking(ctx context.Context, invoiceNumber string, paidAt time.Time) (*repository.PaymentTransition, error) {
	var transition *repository.PaymentTransition

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		payment, booking, err := lockPaymentAndBooking(tx, invoiceNumber)
		if err != nil {
			return err
		}

		transition = &repository.PaymentTransition{
			Payment: toPaymentEntity(payment),
			Booking: paymentBookingEntity(booking),
		}

		if payment.Status == entity.PaymentStatusPaid ||
			payment.Status == entity.PaymentStatusRefundPending ||
			payment.Status == entity.PaymentStatusRefunded {
			transition.LatePayment = payment.Status == entity.PaymentStatusPaid && booking.Status != entity.BookingStatusPendingPayment && booking.Status != entity.BookingStatusPending
			return nil
		}

		paidAt = paidAt.UTC()
		if err := tx.Model(&model.PaymentModel{}).
			Where("id = ?", payment.ID).
			Updates(map[string]interface{}{
				"status":     entity.PaymentStatusPaid,
				"paid_at":    paidAt,
				"last_error": "",
			}).Error; err != nil {
			return paymentDBError(err)
		}
		payment.Status = entity.PaymentStatusPaid
		payment.PaidAt = &paidAt
		payment.LastError = ""
		transition.Payment = toPaymentEntity(payment)
		transition.Applied = true

		if booking.Status == entity.BookingStatusPendingPayment {
			result := tx.Model(&model.BookingModel{}).
				Where("id = ? AND status = ?", booking.ID, entity.BookingStatusPendingPayment).
				Update("status", entity.BookingStatusPending)
			if result.Error != nil {
				return paymentDBError(result.Error)
			}
			if result.RowsAffected > 0 {
				booking.Status = entity.BookingStatusPending
				transition.BookingActivated = true
			}
		} else if booking.Status != entity.BookingStatusPending {
			transition.LatePayment = true
		}

		transition.Booking.Status = booking.Status
		return nil
	})
	if err != nil {
		return nil, err
	}
	return transition, nil
}

func (r *PaymentRepository) MarkExpiredAndExpireBooking(ctx context.Context, invoiceNumber string, _ time.Time) (*repository.PaymentTransition, error) {
	var transition *repository.PaymentTransition

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		payment, booking, err := lockPaymentAndBooking(tx, invoiceNumber)
		if err != nil {
			return err
		}

		transition = &repository.PaymentTransition{
			Payment: toPaymentEntity(payment),
			Booking: paymentBookingEntity(booking),
		}

		switch payment.Status {
		case entity.PaymentStatusPaid, entity.PaymentStatusRefundPending, entity.PaymentStatusRefunded, entity.PaymentStatusExpired:
			return nil
		}

		if err := tx.Model(&model.PaymentModel{}).
			Where("id = ?", payment.ID).
			Update("status", entity.PaymentStatusExpired).Error; err != nil {
			return paymentDBError(err)
		}
		payment.Status = entity.PaymentStatusExpired
		transition.Payment = toPaymentEntity(payment)
		transition.Applied = true

		if booking.Status == entity.BookingStatusPendingPayment {
			result := tx.Model(&model.BookingModel{}).
				Where("id = ? AND status = ?", booking.ID, entity.BookingStatusPendingPayment).
				Update("status", entity.BookingStatusExpired)
			if result.Error != nil {
				return paymentDBError(result.Error)
			}
			if result.RowsAffected > 0 {
				booking.Status = entity.BookingStatusExpired
			}
		}
		transition.Booking.Status = booking.Status
		return nil
	})
	if err != nil {
		return nil, err
	}
	return transition, nil
}

func (r *PaymentRepository) MarkRefundPending(ctx context.Context, bookingID uint) error {
	err := r.db.WithContext(ctx).
		Model(&model.PaymentModel{}).
		Where("booking_id = ? AND status = ?", bookingID, entity.PaymentStatusPaid).
		Update("status", entity.PaymentStatusRefundPending).Error
	if err != nil {
		return paymentDBError(err)
	}
	return nil
}

// lockPaymentAndBooking takes row locks in a stable order (payment then
// booking), which keeps the webhook and expiry worker from deadlocking.
func lockPaymentAndBooking(tx *gorm.DB, invoiceNumber string) (*model.PaymentModel, *model.BookingModel, error) {
	var payment model.PaymentModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("invoice_number = ?", invoiceNumber).
		First(&payment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, paymentNotFoundError()
		}
		return nil, nil, paymentDBError(err)
	}

	var booking model.BookingModel
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&booking, payment.BookingID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
		}
		return nil, nil, paymentDBError(err)
	}

	return &payment, &booking, nil
}

func toPaymentModel(payment *entity.Payment) *model.PaymentModel {
	return &model.PaymentModel{
		ID:                 payment.ID,
		BookingID:          payment.BookingID,
		InvoiceNumber:      payment.InvoiceNumber,
		Amount:             payment.Amount,
		Currency:           payment.Currency,
		Channel:            payment.Channel,
		Status:             payment.Status,
		Provider:           payment.Provider,
		ProviderRequestID:  payment.ProviderRequestID,
		ProviderSessionID:  payment.ProviderSessionID,
		PaymentURL:         payment.PaymentURL,
		ExpiresAt:          payment.ExpiresAt,
		PaidAt:             payment.PaidAt,
		LastError:          payment.LastError,
		IdempotencyKey:     nullableString(payment.IdempotencyKey),
		RequestFingerprint: payment.RequestFingerprint,
		Replayed:           payment.Replayed,
		CreatedAt:          payment.CreatedAt,
		UpdatedAt:          payment.UpdatedAt,
	}
}

func toPaymentEntity(m *model.PaymentModel) *entity.Payment {
	return &entity.Payment{
		ID:                 m.ID,
		BookingID:          m.BookingID,
		InvoiceNumber:      m.InvoiceNumber,
		Amount:             m.Amount,
		Currency:           m.Currency,
		Channel:            m.Channel,
		Status:             m.Status,
		Provider:           m.Provider,
		ProviderRequestID:  m.ProviderRequestID,
		ProviderSessionID:  m.ProviderSessionID,
		PaymentURL:         m.PaymentURL,
		ExpiresAt:          m.ExpiresAt,
		PaidAt:             m.PaidAt,
		LastError:          m.LastError,
		IdempotencyKey:     stringValue(m.IdempotencyKey),
		RequestFingerprint: m.RequestFingerprint,
		Replayed:           m.Replayed,
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
	}
}

// nullableString maps an empty string to a nil pointer, so an unset
// idempotency key is stored as NULL. Every NULL is distinct for a unique
// index, which keeps the index creatable on a database that already holds
// payments.
func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// stringValue maps a NULL column back to the empty string of the entity.
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func paymentBookingEntity(m *model.BookingModel) *entity.Booking {
	return &entity.Booking{
		ID:            m.ID,
		CustomerID:    m.CustomerID,
		ServiceID:     m.ServiceID,
		ProviderID:    m.ProviderID,
		Status:        m.Status,
		PaymentMethod: m.PaymentMethod,
	}
}

func paymentNotFoundError() error {
	return apperror.NewDomainError(apperror.ErrNotFound, "PAYMENT_NOT_FOUND", "payment not found")
}

func paymentDBError(err error) error {
	return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
}

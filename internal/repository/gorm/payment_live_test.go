package gormrepo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

// Live-DB tests for the idempotency columns of the payments table: the key
// must be findable, and it must be nullable so the unique index can be
// created on a database that already holds payments.

// createLiveBooking creates the booking a payment belongs to: the payments
// table has a foreign key on bookings.id.
func createLiveBooking(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	booking := &model.BookingModel{
		CustomerID:    10,
		ProviderID:    20,
		ServiceID:     11,
		Status:        entity.BookingStatusPendingPayment,
		SubTotal:      155000,
		PlatformFee:   5000,
		Total:         160000,
		PaymentMethod: entity.PaymentChannelQRIS,
	}
	require.NoError(t, db.Create(booking).Error)
	return booking.ID
}

func TestLivePayment_FindByIdempotencyKey(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	repo := NewPaymentRepository(db)
	bookingID := createLiveBooking(t, db)

	payment := &entity.Payment{
		BookingID:          bookingID,
		InvoiceNumber:      "SRB-42-7-1",
		Amount:             155000,
		Currency:           "IDR",
		Channel:            "qris",
		Status:             entity.PaymentStatusPending,
		Provider:           "doku",
		IdempotencyKey:     "SRB-42-7",
		RequestFingerprint: "fp-1",
	}
	require.NoError(t, repo.Create(ctx, payment))

	found, err := repo.FindByIdempotencyKey(ctx, "SRB-42-7")
	require.NoError(t, err)
	assert.Equal(t, bookingID, found.BookingID)
	assert.Equal(t, "fp-1", found.RequestFingerprint)
	assert.Equal(t, "SRB-42-7", found.IdempotencyKey)
	assert.False(t, found.Replayed)

	_, err = repo.FindByIdempotencyKey(ctx, "SRB-99-9")
	require.Error(t, err)
}

func TestLivePayment_FindByIdempotencyKey_MissingIsTheDomainNotFound(t *testing.T) {
	db := setupBountyLiveDB(t)
	repo := NewPaymentRepository(db)

	_, err := repo.FindByIdempotencyKey(context.Background(), "SRB-99-9")
	require.Error(t, err)

	var domainErr *apperror.DomainError
	require.True(t, errors.As(err, &domainErr), "expected a domain error, got %T", err)
	assert.Equal(t, apperror.ErrNotFound, domainErr.Err)
	assert.Equal(t, "PAYMENT_NOT_FOUND", domainErr.Code)
}

func TestLivePayment_PaymentWithoutAKeyIsStoredAsNull(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	repo := NewPaymentRepository(db)

	// Two payments without a key must both be storable: the key column is
	// nullable, so it holds NULL and not '', and the unique index stays
	// creatable on a database that already holds payments.
	firstBookingID := createLiveBooking(t, db)
	secondBookingID := createLiveBooking(t, db)

	for _, bookingID := range []uint{firstBookingID, secondBookingID} {
		payment := &entity.Payment{
			BookingID:     bookingID,
			InvoiceNumber: fmt.Sprintf("SRB-%d-1", bookingID),
			Amount:        155000,
			Currency:      "IDR",
			Channel:       "qris",
			Status:        entity.PaymentStatusPending,
			Provider:      "doku",
		}
		require.NoError(t, repo.Create(ctx, payment))
	}

	var nulls int64
	require.NoError(t, db.Model(&model.PaymentModel{}).
		Where("idempotency_key IS NULL").Count(&nulls).Error)
	assert.Equal(t, int64(2), nulls, "an unset key must be stored as NULL, never as ''")

	// The empty key maps back to the empty string on the entity.
	found, err := repo.FindByBookingID(ctx, firstBookingID)
	require.NoError(t, err)
	assert.Equal(t, "", found.IdempotencyKey)
}

func TestLivePayment_DuplicateIdempotencyKeyIsRejected(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	repo := NewPaymentRepository(db)
	firstBookingID := createLiveBooking(t, db)
	secondBookingID := createLiveBooking(t, db)

	first := &entity.Payment{
		BookingID:          firstBookingID,
		InvoiceNumber:      "SRB-42-7-1",
		Amount:             155000,
		Currency:           "IDR",
		Channel:            "qris",
		Status:             entity.PaymentStatusPending,
		Provider:           "doku",
		IdempotencyKey:     "SRB-42-7",
		RequestFingerprint: "fp-1",
	}
	require.NoError(t, repo.Create(ctx, first))

	second := &entity.Payment{
		BookingID:          secondBookingID,
		InvoiceNumber:      "SRB-43-7-1",
		Amount:             155000,
		Currency:           "IDR",
		Channel:            "qris",
		Status:             entity.PaymentStatusPending,
		Provider:           "doku",
		IdempotencyKey:     "SRB-42-7",
		RequestFingerprint: "fp-1",
	}
	require.Error(t, repo.Create(ctx, second), "one key may belong to one payment row only")
}

// legacyPaymentModel is the payments table as it looked before the
// idempotency columns were added.
type legacyPaymentModel struct {
	ID                uint   `gorm:"primaryKey"`
	BookingID         uint   `gorm:"uniqueIndex;not null"`
	InvoiceNumber     string `gorm:"size:64;uniqueIndex;not null"`
	Amount            int64  `gorm:"not null"`
	Currency          string `gorm:"size:3;default:IDR"`
	Channel           string `gorm:"size:20;not null"`
	Status            string `gorm:"size:20;not null;index"`
	Provider          string `gorm:"size:20;not null"`
	ProviderRequestID string `gorm:"size:64"`
	ProviderSessionID string `gorm:"size:128"`
	PaymentURL        string `gorm:"type:text"`
	ExpiresAt         *time.Time
	PaidAt            *time.Time
	LastError         string `gorm:"type:text"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (legacyPaymentModel) TableName() string { return "payments" }

func TestLivePayment_MigrationAddsTheKeyToATableThatAlreadyHoldsPayments(t *testing.T) {
	db := setupBountyLiveDB(t)

	// Rebuild the payments table as it looked before the idempotency
	// columns existed, with two payments already in it.
	require.NoError(t, db.Migrator().DropTable(&model.PaymentModel{}))
	require.NoError(t, db.AutoMigrate(&legacyPaymentModel{}))
	firstBookingID := createLiveBooking(t, db)
	secondBookingID := createLiveBooking(t, db)
	for _, bookingID := range []uint{firstBookingID, secondBookingID} {
		legacy := &legacyPaymentModel{
			BookingID:     bookingID,
			InvoiceNumber: fmt.Sprintf("SRB-%d-7-1", bookingID),
			Amount:        155000,
			Currency:      "IDR",
			Channel:       "qris",
			Status:        entity.PaymentStatusPaid,
			Provider:      "doku",
		}
		require.NoError(t, db.Create(legacy).Error)
	}

	// The migration must succeed: the key column is nullable, so the
	// existing rows get NULL and the unique index can be created. A
	// non-nullable column would fill every row with '' and the index
	// creation would fail here.
	require.NoError(t, db.AutoMigrate(&model.PaymentModel{}))

	var nulls int64
	require.NoError(t, db.Model(&model.PaymentModel{}).
		Where("idempotency_key IS NULL").Count(&nulls).Error)
	assert.Equal(t, int64(2), nulls, "the payments that existed before the migration keep a NULL key")

	var indexDef string
	require.NoError(t, db.Raw(
		`SELECT indexdef FROM pg_indexes WHERE tablename = 'payments' AND indexname LIKE '%idempotency_key%'`,
	).Scan(&indexDef).Error)
	assert.Contains(t, indexDef, "UNIQUE", "the idempotency key must be unique")
}

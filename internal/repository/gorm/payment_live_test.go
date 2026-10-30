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
	err := repo.Create(ctx, second)
	require.Error(t, err, "one key may belong to one payment row only")

	// The failure is the unique violation of the idempotency key index and
	// not some other error: the duplicate key is what the database
	// reports.
	assert.Contains(t, err.Error(), "duplicated key", "the repository must surface the unique violation")

	// Exactly one row holds the key: the rejected payment was not stored.
	var rows int64
	require.NoError(t, db.Model(&model.PaymentModel{}).
		Where("idempotency_key = ?", "SRB-42-7").Count(&rows).Error)
	assert.Equal(t, int64(1), rows, "the rejected payment must not be stored")

	// The database itself names the violation: SQLSTATE 23505 (unique
	// violation) on the idempotency key index. The statement goes to the
	// driver directly, because GORM translates the error and drops the
	// constraint name.
	indexName := liveIdempotencyKeyIndex(t, db)
	sqlDB, dbErr := db.DB()
	require.NoError(t, dbErr)
	_, rawErr := sqlDB.ExecContext(ctx,
		`INSERT INTO payments (booking_id, invoice_number, amount, currency, channel, status, provider, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		secondBookingID, "SRB-43-7-2", 155000, "IDR", "qris", entity.PaymentStatusPending, "doku", "SRB-42-7",
	)
	require.Error(t, rawErr)
	assert.Contains(t, rawErr.Error(), "23505", "a duplicate key must be a unique violation")
	assert.Contains(t, rawErr.Error(), indexName, "the violation must come from the idempotency key index")
}

// liveIdempotencyKeyIndex returns the name of the index on the idempotency
// key column of the payments table.
func liveIdempotencyKeyIndex(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var name string
	require.NoError(t, db.Raw(
		`SELECT indexname FROM pg_indexes WHERE tablename = 'payments' AND indexname LIKE '%idempotency_key%'`,
	).Scan(&name).Error)
	require.NotEmpty(t, name, "the payments table must have an index on the idempotency key")
	return name
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
	ctx := context.Background()
	repo := NewPaymentRepository(db)

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

	// The migration itself must succeed on a table that already holds
	// payments. A key column that is NOT NULL with a default would give
	// every existing row the same value, and the unique index could then
	// not be created; a NOT NULL column without a default could not even
	// be added to a table that holds rows.
	require.NoError(t, db.AutoMigrate(&model.PaymentModel{}))

	// The payments that existed before the migration keep a NULL key.
	var nulls int64
	require.NoError(t, db.Model(&model.PaymentModel{}).
		Where("idempotency_key IS NULL").Count(&nulls).Error)
	assert.Equal(t, int64(2), nulls, "the payments that existed before the migration keep a NULL key")

	// The index on the key is unique.
	indexName := liveIdempotencyKeyIndex(t, db)
	var indexDef string
	require.NoError(t, db.Raw(
		`SELECT indexdef FROM pg_indexes WHERE tablename = 'payments' AND indexname = ?`, indexName,
	).Scan(&indexDef).Error)
	assert.Contains(t, indexDef, "UNIQUE", "the idempotency key must be unique")

	// The decisive part: a payment written after the migration without a
	// key must still be storable. The key column is nullable, so such a
	// row holds NULL and two of them can live next to each other.
	//
	// A column that is not nullable - or one that stores '' for an unset
	// key - would give every keyless payment the same value, and the
	// unique index would reject the second one. A migration that only
	// checks the existing rows cannot see that: ADD COLUMN without NOT
	// NULL leaves those rows NULL as well.
	thirdBookingID := createLiveBooking(t, db)
	fourthBookingID := createLiveBooking(t, db)
	for _, bookingID := range []uint{thirdBookingID, fourthBookingID} {
		keyless := &entity.Payment{
			BookingID:     bookingID,
			InvoiceNumber: fmt.Sprintf("SRB-%d-7-2", bookingID),
			Amount:        155000,
			Currency:      "IDR",
			Channel:       "qris",
			Status:        entity.PaymentStatusPending,
			Provider:      "doku",
		}
		require.NoError(t, repo.Create(ctx, keyless),
			"a payment without an idempotency key must be storable after the migration")
	}

	var keyless int64
	require.NoError(t, db.Model(&model.PaymentModel{}).
		Where("idempotency_key IS NULL").Count(&keyless).Error)
	assert.Equal(t, int64(4), keyless, "every payment without a key holds NULL, never ''")
}

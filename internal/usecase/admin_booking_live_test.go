package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	gormrepo "github.com/serabutin/serabutin-id-backend/internal/repository/gorm"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

// Live-DB regression test for the admin force-cancel 500: the force-cancel
// path writes PolicyVersion "admin-override" (14 chars) and Reason
// "admin_override:"+reason (15-char prefix) into narrow varchar columns.
// Uses the REAL gorm repositories against testcontainers Postgres — fakes
// enforce no column widths, which is why unit tests stayed green while prod
// returned 500 (value too long for type character varying).
func setupAdminCancelLiveDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := testpostgres.Run(ctx,
		"postgres:16-alpine",
		testpostgres.WithDatabase("testdb"),
		testpostgres.WithUsername("test"),
		testpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := gorm.Open(gormpostgres.Open(connStr), &gorm.Config{TranslateError: true})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.BookingModel{}, &model.BookingAddOnModel{}, &model.CancellationModel{}))

	// Simulate an existing deployment created before the widening: legacy
	// narrow widths that AutoMigrate will NOT resize on its own.
	require.NoError(t, db.Exec(`ALTER TABLE cancellations ALTER COLUMN policy_version TYPE varchar(10)`).Error)
	require.NoError(t, db.Exec(`ALTER TABLE cancellations ALTER COLUMN reason TYPE varchar(30)`).Error)

	// Boot widening guards (must mirror cmd/server/main.go). Idempotent:
	// run twice to prove re-boot safety.
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Exec(`ALTER TABLE cancellations ALTER COLUMN policy_version TYPE varchar(32)`).Error)
		require.NoError(t, db.Exec(`ALTER TABLE cancellations ALTER COLUMN reason TYPE varchar(500)`).Error)
	}
	return db
}

func seedLiveBooking(t *testing.T, ctx context.Context, bookingRepo *gormrepo.BookingRepository, status string) uint {
	t.Helper()
	b := &entity.Booking{
		CustomerID: 10, ProviderID: 20, ServiceID: 11,
		Status: status, SubTotal: 75000, PlatformFee: 5000, Total: 80000,
		PaymentMethod: "cash",
	}
	require.NoError(t, bookingRepo.Create(ctx, b))
	require.NotZero(t, b.ID)
	return b.ID
}

func TestAdminCancelBookingLive_LongestValuesRoundTrip(t *testing.T) {
	db := setupAdminCancelLiveDB(t)
	ctx := context.Background()
	cancelRepo := gormrepo.NewCancellationRepository(db)

	// Longest realistic row: admin-override policy + near-max reason.
	longReason := "admin_override:" + strings.Repeat("dugaan penipuan terkonfirmasi oleh tim moderasi. ", 9)
	require.LessOrEqual(t, len(longReason), 500)
	c := &entity.Cancellation{
		BookingID: 1, CancelledBy: 7, CancelledByRole: "admin",
		Reason: longReason, FeeCharged: 0,
		PlatformFeeSnapshot: 5000, SubtotalSnapshot: 75000,
		PolicyVersion: AdminCancellationPolicyVersion,
	}
	require.NoError(t, cancelRepo.Create(ctx, c))
	require.NotZero(t, c.ID)

	got, err := cancelRepo.FindByBookingID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, longReason, got.Reason)
	assert.Equal(t, AdminCancellationPolicyVersion, got.PolicyVersion)
	assert.Equal(t, "admin", got.CancelledByRole)
}

func TestAdminCancelBookingLive_EndToEndWritesRow(t *testing.T) {
	db := setupAdminCancelLiveDB(t)
	ctx := context.Background()
	bookingRepo := gormrepo.NewBookingRepository(db)
	cancelRepo := gormrepo.NewCancellationRepository(db)
	uc := NewAdminBookingUsecase(bookingRepo, cancelRepo)

	id := seedLiveBooking(t, ctx, bookingRepo, "confirmed")
	// 200-char admin free-text reason: stored value is 15-char prefix + 200.
	adminReason := strings.Repeat("sengketa pembayaran perlu dibatalkan paksa. ", 5)[:200]
	booking, cancellation, err := uc.AdminCancelBooking(ctx, 7, id, adminReason)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", booking.Status)
	assert.Equal(t, "admin_override:"+adminReason, cancellation.Reason)
	assert.Equal(t, AdminCancellationPolicyVersion, cancellation.PolicyVersion)
	require.NotZero(t, cancellation.ID)

	// Row really persisted via the real repo path.
	stored, err := cancelRepo.FindByBookingID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, cancellation.ID, stored.ID)
	assert.Equal(t, "admin_override:"+adminReason, stored.Reason)
}

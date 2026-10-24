package usecase

import (
	"context"
	"errors"
	"net/http"
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
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	gormrepo "github.com/serabutin/serabutin-id-backend/internal/repository/gorm"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
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
	require.NoError(t, db.AutoMigrate(&model.BookingModel{}, &model.BookingAddOnModel{}, &model.PaymentModel{}, &model.CancellationModel{}))

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

func TestAdminCancelBookingLive_ConcurrentForceCancel(t *testing.T) {
	db := setupAdminCancelLiveDB(t)
	ctx := context.Background()
	bookingRepo := gormrepo.NewBookingRepository(db)
	cancelRepo := gormrepo.NewCancellationRepository(db)
	uc := NewAdminBookingUsecase(bookingRepo, cancelRepo)

	id := 0
	// Race several bookings so the INSERT-level collision (not just the
	// pre-check path) is exercised live; every race must end 1×200 + 1×422
	// with zero 500s. The unique index guarantees at most one winner, and
	// the F7 mapping turns the loser's unique violation into the
	// terminal-state outcome.
	var succeeded, terminal int
	for i := 0; i < 10; i++ {
		id = int(seedLiveBooking(t, ctx, bookingRepo, "pending"))
		start := make(chan struct{})
		errs := make(chan error, 2)
		for j := 0; j < 2; j++ {
			go func() {
				<-start
				_, _, err := uc.AdminCancelBooking(ctx, 7, uint(id), "sengketa ganda")
				errs <- err
			}()
		}
		close(start)
		for j := 0; j < 2; j++ {
			err := <-errs
			if err == nil {
				succeeded++
				continue
			}
			assert.NotErrorIs(t, err, apperror.ErrInternalServer, "loser must not see a 500")
			assert.Equal(t, http.StatusUnprocessableEntity, apierror.FromDomainError(err).HTTPCode)
			var domainErr *apperror.DomainError
			if assert.ErrorAs(t, err, &domainErr) {
				assert.Equal(t, "INVALID_STATUS_TRANSITION", domainErr.Code)
				terminal++
			}
		}
	}
	assert.Equal(t, 10, succeeded, "exactly one force-cancel wins each race")
	assert.Equal(t, 10, terminal, "each loser reports 422 INVALID_STATUS_TRANSITION")

	var n int64
	require.NoError(t, db.WithContext(ctx).Model(&model.CancellationModel{}).Where("booking_id = ?", id).Count(&n).Error)
	assert.Equal(t, int64(1), n)

	// Deterministic cover for the unique-violation mapping: a sequential
	// second attempt at the repo layer hits the booking_id unique index
	// and must surface CANCEL_GUARD_FAILED, not DB_ERROR.
	retry := &entity.Cancellation{
		BookingID: uint(id), CancelledBy: 7, CancelledByRole: "admin",
		Reason: "admin_override:duplikat", PolicyVersion: AdminCancellationPolicyVersion,
	}
	rerr := cancelRepo.CreateAndApplyCancel(ctx, retry, 7)
	require.Error(t, rerr)
	assert.NotErrorIs(t, rerr, apperror.ErrInternalServer)
	var rdomainErr *apperror.DomainError
	if assert.ErrorAs(t, rerr, &rdomainErr) {
		assert.Equal(t, "CANCEL_GUARD_FAILED", rdomainErr.Code)
	}
	assert.True(t, errors.Is(rerr, apperror.ErrInvalidTransition))
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

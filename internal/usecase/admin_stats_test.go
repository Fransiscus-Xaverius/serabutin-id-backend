package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// Ticket 7: platform-wide admin dashboard stats — exact aggregates over
// seeded counts, zeros (never nulls) on empty, cancel-rate math.

type adminStatsFakeStore struct {
	users       repository.AdminUserStats
	bookings    repository.AdminBookingStats
	services    repository.AdminServiceStats
	openReports int64
	err         error
}

func (f *adminStatsFakeStore) UserStats(ctx context.Context, newSince time.Time) (repository.AdminUserStats, error) {
	return f.users, f.err
}

func (f *adminStatsFakeStore) BookingStats(ctx context.Context, since time.Time) (repository.AdminBookingStats, error) {
	return f.bookings, f.err
}

func (f *adminStatsFakeStore) ServiceStats(ctx context.Context) (repository.AdminServiceStats, error) {
	return f.services, f.err
}

func (f *adminStatsFakeStore) OpenReportCount(ctx context.Context) (int64, error) {
	return f.openReports, f.err
}

func TestAdminStats_ReturnsExactAggregates(t *testing.T) {
	uc := NewAdminStatsUsecase(&adminStatsFakeStore{
		users: repository.AdminUserStats{
			Total:  120,
			ByRole: map[string]int64{"customer": 90, "provider": 28, "admin": 2},
			New7d:  9,
		},
		bookings: repository.AdminBookingStats{
			ByStatus:     map[string]int64{"pending": 4, "accepted": 6, "completed": 20, "cancelled": 3},
			Total30d:     33,
			Cancelled30d: 3,
			GMV30d:       2400000,
			Fees30d:      160000,
		},
		services:    repository.AdminServiceStats{PendingCount: 5, ActiveCount: 42},
		openReports: 2,
	})

	stats, err := uc.GetAdminStats(context.Background())
	if err != nil {
		t.Fatalf("GetAdminStats: %v", err)
	}
	if stats.Users.Total != 120 || stats.Users.New7d != 9 {
		t.Errorf("users = %+v, want total 120 new_7d 9", stats.Users)
	}
	if stats.Users.ByRole["provider"] != 28 || stats.Users.ByRole["admin"] != 2 {
		t.Errorf("by_role = %v, want provider 28 admin 2", stats.Users.ByRole)
	}
	if stats.Bookings.Total30d != 33 {
		t.Errorf("total_30d = %d, want 33", stats.Bookings.Total30d)
	}
	if stats.Bookings.ByStatus["completed"] != 20 {
		t.Errorf("by_status = %v, want completed 20", stats.Bookings.ByStatus)
	}
	wantRate := 3.0 / 33.0
	if stats.Bookings.CancelRate30d != wantRate {
		t.Errorf("cancel_rate = %v, want %v", stats.Bookings.CancelRate30d, wantRate)
	}
	if stats.GMV.Total30d != 2400000 || stats.GMV.PlatformFees30d != 160000 {
		t.Errorf("gmv = %+v, want total 2400000 fees 160000", stats.GMV)
	}
	if stats.Services.PendingCount != 5 || stats.Services.ActiveCount != 42 {
		t.Errorf("services = %+v, want pending 5 active 42", stats.Services)
	}
	if stats.Reports.OpenCount != 2 {
		t.Errorf("open_count = %d, want 2", stats.Reports.OpenCount)
	}
}

func TestAdminStats_EmptyPlatformYieldsZerosNotNulls(t *testing.T) {
	uc := NewAdminStatsUsecase(&adminStatsFakeStore{})

	stats, err := uc.GetAdminStats(context.Background())
	if err != nil {
		t.Fatalf("GetAdminStats: %v", err)
	}
	if stats.Users.Total != 0 || stats.Users.New7d != 0 {
		t.Errorf("users = %+v, want zeros", stats.Users)
	}
	if stats.Users.ByRole == nil || len(stats.Users.ByRole) != 0 {
		t.Errorf("by_role = %v, want non-nil empty map", stats.Users.ByRole)
	}
	if stats.Bookings.ByStatus == nil || len(stats.Bookings.ByStatus) != 0 {
		t.Errorf("by_status = %v, want non-nil empty map", stats.Bookings.ByStatus)
	}
	if stats.Bookings.Total30d != 0 || stats.Bookings.CancelRate30d != 0 {
		t.Errorf("bookings = %+v, want zeros (no div-by-zero)", stats.Bookings)
	}
	if stats.GMV.Total30d != 0 || stats.GMV.PlatformFees30d != 0 {
		t.Errorf("gmv = %+v, want zeros", stats.GMV)
	}
	if stats.Services.PendingCount != 0 || stats.Services.ActiveCount != 0 {
		t.Errorf("services = %+v, want zeros", stats.Services)
	}
	if stats.Reports.OpenCount != 0 {
		t.Errorf("open_count = %d, want 0", stats.Reports.OpenCount)
	}
}

func TestAdminStats_StoreErrorPropagates(t *testing.T) {
	boom := errors.New("db down")
	uc := NewAdminStatsUsecase(&adminStatsFakeStore{err: boom})

	if _, err := uc.GetAdminStats(context.Background()); err == nil {
		t.Error("expected store error to propagate")
	}
}

package repository

import (
	"context"
	"time"
)

// AdminUserStats is the platform-wide user rollup for GET /admin/stats.
// Maps are never nil — an empty platform yields empty maps and zero counts.
type AdminUserStats struct {
	Total  int64
	ByRole map[string]int64
	New7d  int64
}

// AdminBookingStats is the trailing-30-day booking rollup. Cancelled30d is
// the cancelled slice of Total30d; the cancel rate is derived in the
// usecase. GMV sums Total over non-cancelled bookings in the window.
type AdminBookingStats struct {
	ByStatus    map[string]int64
	Total30d    int64
	Cancelled30d int64
	GMV30d      int64
	Fees30d     int64
}

// AdminServiceStats carries the moderation-relevant service counts.
type AdminServiceStats struct {
	PendingCount int64
	ActiveCount  int64
}

// AdminStatsRepository is the read-only aggregation port behind
// GET /admin/stats. Implementations must use efficient COUNT / GROUP BY /
// SUM queries — never full-table loads. Soft-deleted rows are excluded.
type AdminStatsRepository interface {
	UserStats(ctx context.Context, newSince time.Time) (AdminUserStats, error)
	BookingStats(ctx context.Context, since time.Time) (AdminBookingStats, error)
	ServiceStats(ctx context.Context) (AdminServiceStats, error)
	OpenReportCount(ctx context.Context) (int64, error)
}

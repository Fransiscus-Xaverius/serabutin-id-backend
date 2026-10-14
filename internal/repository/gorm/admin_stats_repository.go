package gormrepo

import (
	"context"
	"time"

	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

// AdminStatsRepository serves the read-only GET /admin/stats aggregations.
// Every method issues COUNT / GROUP BY / SUM queries only — no row materialization.
type AdminStatsRepository struct {
	db *gorm.DB
}

func NewAdminStatsRepository(db *gorm.DB) *AdminStatsRepository {
	return &AdminStatsRepository{db: db}
}

type statusCountRow struct {
	Status string
	Count  int64
}

type roleCountRow struct {
	Role  string
	Count int64
}

func dbError(op string, err error) error {
	return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", op+": "+err.Error())
}

// UserStats counts non-deleted users in total, grouped by role, plus
// signups with created_at >= newSince. GORM's soft-delete scope excludes
// deleted rows automatically.
func (r *AdminStatsRepository) UserStats(ctx context.Context, newSince time.Time) (repository.AdminUserStats, error) {
	out := repository.AdminUserStats{ByRole: map[string]int64{}}
	var total int64
	if err := r.db.WithContext(ctx).Model(&model.UserModel{}).Count(&total).Error; err != nil {
		return out, dbError("count users", err)
	}
	var rows []roleCountRow
	if err := r.db.WithContext(ctx).Model(&model.UserModel{}).
		Select("role, COUNT(*) AS count").Group("role").Scan(&rows).Error; err != nil {
		return out, dbError("count users by role", err)
	}
	for _, row := range rows {
		out.ByRole[row.Role] = row.Count
	}
	var fresh int64
	if err := r.db.WithContext(ctx).Model(&model.UserModel{}).
		Where("created_at >= ?", newSince).Count(&fresh).Error; err != nil {
		return out, dbError("count new users", err)
	}
	out.Total = total
	out.New7d = fresh
	return out, nil
}

// BookingStats aggregates bookings with created_at >= since: counts grouped
// by status, plus GMV (SUM(total)) and fees (SUM(platform_fee)) over the
// non-cancelled slice of the same window.
func (r *AdminStatsRepository) BookingStats(ctx context.Context, since time.Time) (repository.AdminBookingStats, error) {
	out := repository.AdminBookingStats{ByStatus: map[string]int64{}}
	var rows []statusCountRow
	if err := r.db.WithContext(ctx).Model(&model.BookingModel{}).
		Select("status, COUNT(*) AS count").Where("created_at >= ?", since).
		Group("status").Scan(&rows).Error; err != nil {
		return out, dbError("count bookings by status", err)
	}
	for _, row := range rows {
		out.ByStatus[row.Status] = row.Count
		out.Total30d += row.Count
		if row.Status == "cancelled" {
			out.Cancelled30d = row.Count
		}
	}
	type sumRow struct {
		GMV  int64
		Fees int64
	}
	var sums sumRow
	if err := r.db.WithContext(ctx).Model(&model.BookingModel{}).
		Select("COALESCE(SUM(total),0) AS gmv, COALESCE(SUM(platform_fee),0) AS fees").
		Where("created_at >= ? AND status <> ?", since, "cancelled").
		Scan(&sums).Error; err != nil {
		return out, dbError("sum booking gmv", err)
	}
	out.GMV30d = sums.GMV
	out.Fees30d = sums.Fees
	return out, nil
}

// ServiceStats counts non-deleted services in the moderation-relevant
// statuses. Soft-deleted rows are excluded by GORM's delete scope.
func (r *AdminStatsRepository) ServiceStats(ctx context.Context) (repository.AdminServiceStats, error) {
	var out repository.AdminServiceStats
	if err := r.db.WithContext(ctx).Model(&model.ServiceModel{}).
		Where("status = ?", "pending").Count(&out.PendingCount).Error; err != nil {
		return out, dbError("count pending services", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.ServiceModel{}).
		Where("status = ?", "active").Count(&out.ActiveCount).Error; err != nil {
		return out, dbError("count active services", err)
	}
	return out, nil
}

// OpenReportCount counts reports still awaiting moderation.
func (r *AdminStatsRepository) OpenReportCount(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.ReportModel{}).
		Where("status = ?", "open").Count(&count).Error; err != nil {
		return 0, dbError("count open reports", err)
	}
	return count, nil
}

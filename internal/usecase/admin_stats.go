package usecase

import (
	"context"
	"log"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"golang.org/x/sync/errgroup"
)

// AdminStatsUsecase owns the read-only platform rollups behind
// GET /admin/stats. The four aggregation axes fan out with errgroup over a
// 10s timeout context; the response carries no PII. Caching is
// deliberately left out: GETs are never audit-logged and the COUNT queries
// are index-backed, so a short-TTL cache can be layered later without
// changing this shape.
type AdminStatsUsecase struct {
	statsRepo repository.AdminStatsRepository
}

func NewAdminStatsUsecase(statsRepo repository.AdminStatsRepository) *AdminStatsUsecase {
	return &AdminStatsUsecase{statsRepo: statsRepo}
}

// GetAdminStats returns platform health aggregates. Windows use server-local
// UTC day boundaries (midnight UTC). Maps are always initialized — an empty
// platform yields zeros, never nulls.
func (uc *AdminStatsUsecase) GetAdminStats(ctx context.Context) (*dto.AdminStatsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	now := time.Now().UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	newSince := day.AddDate(0, 0, -7)
	since30d := day.AddDate(0, 0, -30)

	var users repository.AdminUserStats
	var bookings repository.AdminBookingStats
	var services repository.AdminServiceStats
	var openReports int64

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		users, err = uc.statsRepo.UserStats(gctx, newSince)
		if err != nil {
			log.Printf("[AdminStatsUsecase.GetAdminStats] user stats error: %v", err)
		}
		return err
	})
	g.Go(func() error {
		var err error
		bookings, err = uc.statsRepo.BookingStats(gctx, since30d)
		if err != nil {
			log.Printf("[AdminStatsUsecase.GetAdminStats] booking stats error: %v", err)
		}
		return err
	})
	g.Go(func() error {
		var err error
		services, err = uc.statsRepo.ServiceStats(gctx)
		if err != nil {
			log.Printf("[AdminStatsUsecase.GetAdminStats] service stats error: %v", err)
		}
		return err
	})
	g.Go(func() error {
		var err error
		openReports, err = uc.statsRepo.OpenReportCount(gctx)
		if err != nil {
			log.Printf("[AdminStatsUsecase.GetAdminStats] report stats error: %v", err)
		}
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var cancelRate float64
	if bookings.Total30d > 0 {
		cancelRate = float64(bookings.Cancelled30d) / float64(bookings.Total30d)
	}
	if users.ByRole == nil {
		users.ByRole = map[string]int64{}
	}
	if bookings.ByStatus == nil {
		bookings.ByStatus = map[string]int64{}
	}

	return &dto.AdminStatsResponse{
		Users: dto.AdminStatsUsersResponse{
			Total:  users.Total,
			ByRole: users.ByRole,
			New7d:  users.New7d,
		},
		Bookings: dto.AdminStatsBookingsResponse{
			ByStatus:      bookings.ByStatus,
			Total30d:      bookings.Total30d,
			CancelRate30d: cancelRate,
		},
		GMV: dto.AdminStatsGMVResponse{
			Total30d:        bookings.GMV30d,
			PlatformFees30d: bookings.Fees30d,
		},
		Services: dto.AdminStatsServicesResponse{
			PendingCount: services.PendingCount,
			ActiveCount:  services.ActiveCount,
		},
		Reports: dto.AdminStatsReportsResponse{
			OpenCount: openReports,
		},
	}, nil
}

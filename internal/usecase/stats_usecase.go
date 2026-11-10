package usecase

import (
	"context"
	"log"

	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

type StatsUsecase struct {
	userRepo    repository.UserRepository
	serviceRepo repository.ServiceRepository
	bookingRepo repository.BookingRepository
}

func NewStatsUsecase(userRepo repository.UserRepository, serviceRepo repository.ServiceRepository, bookingRepo repository.BookingRepository) *StatsUsecase {
	return &StatsUsecase{userRepo: userRepo, serviceRepo: serviceRepo, bookingRepo: bookingRepo}
}

func (uc *StatsUsecase) GetUserStats(ctx context.Context, userID uint) (*dto.UserStatsResponse, error) {
	user, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		log.Printf("[StatsUsecase.GetUserStats] find user error: %v", err)
		return nil, err
	}

	// Jobs posted = requested-type services owned by the user (non-deleted).
	posted, _, err := uc.serviceRepo.FindByProviderID(ctx, userID, repository.ServiceFilter{
		ServiceType: "requested",
		Page:        1,
		Limit:       1000,
	})
	if err != nil {
		log.Printf("[StatsUsecase.GetUserStats] find services error: %v", err)
		return nil, err
	}
	var active int64
	postedIDs := make(map[uint]bool, len(posted))
	for _, s := range posted {
		postedIDs[s.ID] = true
		if s.Status == "active" {
			active++
		}
	}

	// Bookings where the user is the customer.
	bookings, _, err := uc.bookingRepo.FindByUserID(ctx, userID, "customer", repository.BookingFilter{Page: 1, Limit: 1000})
	if err != nil {
		log.Printf("[StatsUsecase.GetUserStats] find bookings error: %v", err)
		return nil, err
	}
	hiredServices := make(map[uint]bool)
	var spent int64
	for _, b := range bookings {
		if b.Status == "cancelled" || b.Status == "rejected" {
			continue
		}
		if postedIDs[b.ServiceID] {
			hiredServices[b.ServiceID] = true
		}
		if b.Status == "completed" {
			spent += b.Total
		}
	}

	var hireRate float64
	if len(posted) > 0 {
		hireRate = float64(len(hiredServices)) / float64(len(posted))
	}

	return &dto.UserStatsResponse{
		User: dto.PublicUserResponse{
			ID:        user.ID,
			Name:      user.Name,
			AvatarURL: user.AvatarURL,
		},
		Role:           user.Role,
		MemberSince:    user.CreatedAt.Format("2006-01-02"),
		JobsPosted:     int64(len(posted)),
		ActiveListings: active,
		HireRate:       hireRate,
		TotalSpent:     spent,
	}, nil
}

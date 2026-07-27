package usecase

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

type BidUsecase struct {
	bidRepo     repository.BidRepository
	serviceRepo repository.ServiceRepository
	bookingRepo repository.BookingRepository
	userRepo    repository.UserRepository
	platformFee int64
}

func NewBidUsecase(
	bidRepo repository.BidRepository,
	serviceRepo repository.ServiceRepository,
	bookingRepo repository.BookingRepository,
	userRepo repository.UserRepository,
	platformFee int64,
) *BidUsecase {
	return &BidUsecase{
		bidRepo:     bidRepo,
		serviceRepo: serviceRepo,
		bookingRepo: bookingRepo,
		userRepo:    userRepo,
		platformFee: platformFee,
	}
}

func (uc *BidUsecase) CreateBid(ctx context.Context, providerID uint, req dto.CreateBidRequest) (*entity.Bid, error) {
	service, err := uc.serviceRepo.FindByID(ctx, req.ServiceID)
	if err != nil {
		log.Printf("[BidUsecase.CreateBid] find service error: %v", err)
		return nil, err
	}

	if service.Status != "active" {
		return nil, apperror.NewDomainError(apperror.ErrServiceInactive, "SERVICE_INACTIVE", "service is not active")
	}

	if service.ProviderID == providerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "SELF_BID", "cannot bid on your own service")
	}

	bid := &entity.Bid{
		ServiceID:  req.ServiceID,
		ProviderID: providerID,
		Price:      req.Price,
		Notes:      req.Notes,
		Status:     "pending",
	}

	if err := uc.bidRepo.Create(ctx, bid); err != nil {
		log.Printf("[BidUsecase.CreateBid] create error: %v", err)
		return nil, err
	}

	// Auto-accept (spec 002): if the bidder's profile has a budget_min
	// set and the bid price is at or below it, immediately transition
	// the bid to accepted and create the booking — no human in the loop.
	if err := uc.maybeAutoAccept(ctx, bid, service); err != nil {
		// Non-fatal: log and return the bid in its pending state. The
		// customer can still accept manually.
		log.Printf("[BidUsecase.CreateBid] auto-accept skipped: %v", err)
	}

	return bid, nil
}

func (uc *BidUsecase) maybeAutoAccept(ctx context.Context, bid *entity.Bid, service *entity.Service) error {
	provider, err := uc.userRepo.FindByID(ctx, bid.ProviderID)
	if err != nil {
		return err
	}
	if !shouldAutoAccept(bid.Price, provider.BudgetMin, provider.BudgetMax) {
		return nil
	}
	return uc.completeAccept(ctx, bid, service)
}

// completeAccept transitions a pending bid to accepted and creates the
// booking in one shot. Shared between the manual AcceptBid path and the
// auto-accept path in CreateBid. Uses AcceptIfPending for race safety:
// if a concurrent path already accepted/rejected this bid, we silently
// skip — the caller already received the bid in pending state and may
// retry manually.
func (uc *BidUsecase) completeAccept(ctx context.Context, bid *entity.Bid, service *entity.Service) error {
	if err := uc.bidRepo.AcceptIfPending(ctx, bid.ID); err != nil {
		// Race lost: someone else moved the bid off pending. Skip
		// booking creation; do not propagate the error to the caller.
		if errors.Is(err, apperror.ErrInvalidTransition) {
			return nil
		}
		return err
	}
	if err := uc.bidRepo.RejectOthers(ctx, bid.ServiceID, bid.ID); err != nil {
		return err
	}
	total := bid.Price + uc.platformFee
	booking := &entity.Booking{
		CustomerID:    service.ProviderID,
		ServiceID:     bid.ServiceID,
		ProviderID:    bid.ProviderID,
		ScheduleDate:  time.Now().Add(24 * time.Hour),
		ScheduleTime:  "09:00",
		Address:       "",
		Notes:         bid.Notes,
		SubTotal:      bid.Price,
		PlatformFee:   uc.platformFee,
		Total:         total,
		PaymentMethod: "cash_on_service",
		Status:        "pending",
	}
	return uc.bookingRepo.Create(ctx, booking)
}

func (uc *BidUsecase) GetBidsForService(ctx context.Context, userID uint, serviceID uint) ([]*entity.Bid, error) {
	service, err := uc.serviceRepo.FindByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	if service.ProviderID != userID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the service owner can view bids")
	}

	bids, err := uc.bidRepo.FindByServiceID(ctx, serviceID)
	if err != nil {
		log.Printf("[BidUsecase.GetBidsForService] find error: %v", err)
		return nil, err
	}

	return bids, nil
}

func (uc *BidUsecase) GetMyBids(ctx context.Context, providerID uint, filter repository.BidFilter) ([]*entity.Bid, int64, error) {
	bids, total, err := uc.bidRepo.FindByProviderID(ctx, providerID, filter)
	if err != nil {
		log.Printf("[BidUsecase.GetMyBids] find error: %v", err)
		return nil, 0, err
	}
	return bids, total, nil
}

func (uc *BidUsecase) AcceptBid(ctx context.Context, userID uint, bidID uint) (*entity.Booking, error) {
	bid, err := uc.bidRepo.FindByID(ctx, bidID)
	if err != nil {
		return nil, err
	}

	service, err := uc.serviceRepo.FindByID(ctx, bid.ServiceID)
	if err != nil {
		return nil, err
	}

	if service.ProviderID != userID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the service owner can accept bids")
	}

	if bid.Status != "pending" {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only accept pending bids")
	}

	// Race-safe accept: if a concurrent path moved the bid off pending,
	// AcceptIfPending returns ErrInvalidTransition — propagate that
	// rather than creating a duplicate booking.
	if err := uc.bidRepo.AcceptIfPending(ctx, bidID); err != nil {
		return nil, err
	}

	uc.bidRepo.RejectOthers(ctx, bid.ServiceID, bidID)

	total := bid.Price + uc.platformFee
	booking := &entity.Booking{
		CustomerID:    userID,
		ServiceID:     bid.ServiceID,
		ProviderID:    bid.ProviderID,
		ScheduleDate:  time.Now().Add(24 * time.Hour),
		ScheduleTime:  "09:00",
		Address:       "",
		Notes:         bid.Notes,
		SubTotal:      bid.Price,
		PlatformFee:   uc.platformFee,
		Total:         total,
		PaymentMethod: "cash_on_service",
		Status:        "pending",
	}

	if err := uc.bookingRepo.Create(ctx, booking); err != nil {
		return nil, err
	}

	return booking, nil
}

func (uc *BidUsecase) RejectBid(ctx context.Context, userID uint, bidID uint) error {
	bid, err := uc.bidRepo.FindByID(ctx, bidID)
	if err != nil {
		return err
	}

	service, err := uc.serviceRepo.FindByID(ctx, bid.ServiceID)
	if err != nil {
		return err
	}

	if service.ProviderID != userID {
		return apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the service owner can reject bids")
	}

	if bid.Status != "pending" {
		return apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only reject pending bids")
	}

	return uc.bidRepo.UpdateStatus(ctx, bidID, "rejected")
}

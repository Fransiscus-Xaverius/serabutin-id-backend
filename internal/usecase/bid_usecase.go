package usecase

import (
	"context"
	"errors"
	"fmt"
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
	notifier    Notifier
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

func (uc *BidUsecase) SetNotifier(n Notifier) {
	uc.notifier = n
}

// notifyBidBoth writes one notification row per bid party (bidder + service
// owner). Best-effort: failures are logged and never fail the transition.
func (uc *BidUsecase) notifyBidBoth(ctx context.Context, bidderID, ownerID uint, typ, title, body string, bidID uint) {
	if uc.notifier == nil {
		return
	}
	data := fmt.Sprintf(`{"bid_id":%d}`, bidID)
	if _, err := uc.notifier.Notify(ctx, bidderID, typ, title, body, data); err != nil {
		log.Printf("[BidUsecase.notify] bidder %d: %v", bidderID, err)
	}
	if ownerID != bidderID {
		if _, err := uc.notifier.Notify(ctx, ownerID, typ, title, body, data); err != nil {
			log.Printf("[BidUsecase.notify] owner %d: %v", ownerID, err)
		}
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
	accepted, err := uc.maybeAutoAccept(ctx, bid, service)
	if err != nil {
		// Non-fatal: log and return the bid in its pending state. The
		// customer can still accept manually.
		log.Printf("[BidUsecase.CreateBid] auto-accept skipped: %v", err)
	}
	if accepted {
		bid.Status = "accepted"
		uc.notifyBidBoth(ctx, providerID, service.ProviderID, "bid.accepted", "Penawaran diterima otomatis", "Penawaran Anda memenuhi harga auto-accept dan booking dibuat.", bid.ID)
	} else {
		uc.notifyBidBoth(ctx, providerID, service.ProviderID, "bid.created", "Penawaran baru", "Mitra mengajukan penawaran untuk lowongan Anda.", bid.ID)
	}

	return bid, nil
}

func (uc *BidUsecase) maybeAutoAccept(ctx context.Context, bid *entity.Bid, service *entity.Service) (bool, error) {
	if service.ServiceType == "requested" && shouldAutoAcceptJob(bid.Price, service.AutoAcceptPrice) {
		return uc.completeAccept(ctx, bid, service)
	}

	provider, err := uc.userRepo.FindByID(ctx, bid.ProviderID)
	if err != nil {
		return false, err
	}
	if !shouldAutoAccept(bid.Price, provider.BudgetMin, provider.BudgetMax) {
		return false, nil
	}
	return uc.completeAccept(ctx, bid, service)
}

// completeAccept transitions a pending bid to accepted and creates the
// booking in one shot. Shared between the manual AcceptBid path and the
// auto-accept path in CreateBid. Uses AcceptIfPending for race safety:
// if a concurrent path already accepted/rejected this bid, we silently
// skip — the caller already received the bid in pending state and may
// retry manually.
func (uc *BidUsecase) completeAccept(ctx context.Context, bid *entity.Bid, service *entity.Service) (bool, error) {
	if err := uc.bidRepo.AcceptIfPending(ctx, bid.ID); err != nil {
		// Race lost: someone else moved the bid off pending. Skip
		// booking creation; do not propagate the error to the caller.
		if errors.Is(err, apperror.ErrInvalidTransition) {
			return false, nil
		}
		return false, err
	}
	if err := uc.bidRepo.RejectOthers(ctx, bid.ServiceID, bid.ID); err != nil {
		return false, err
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
	if err := uc.bookingRepo.Create(ctx, booking); err != nil {
		return false, err
	}
	return true, nil
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

	uc.notifyBidBoth(ctx, bid.ProviderID, userID, "bid.accepted", "Penawaran diterima", "Penawaran Anda diterima dan booking dibuat.", bid.ID)
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

	if err := uc.bidRepo.UpdateStatus(ctx, bidID, "rejected"); err != nil {
		return err
	}
	uc.notifyBidBoth(ctx, bid.ProviderID, userID, "bid.rejected", "Penawaran ditolak", "Penawaran Anda ditolak.", bid.ID)
	return nil
}

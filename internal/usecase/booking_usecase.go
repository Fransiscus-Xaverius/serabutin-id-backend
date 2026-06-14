package usecase

import (
	"context"
	"log"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

type BookingUsecase struct {
	bookingRepo repository.BookingRepository
	serviceRepo repository.ServiceRepository
	userRepo    repository.UserRepository
	platformFee int64
}

func NewBookingUsecase(
	bookingRepo repository.BookingRepository,
	serviceRepo repository.ServiceRepository,
	userRepo repository.UserRepository,
	platformFee int64,
) *BookingUsecase {
	return &BookingUsecase{
		bookingRepo: bookingRepo,
		serviceRepo: serviceRepo,
		userRepo:    userRepo,
		platformFee: platformFee,
	}
}

func (uc *BookingUsecase) CreateBooking(ctx context.Context, customerID uint, req dto.CreateBookingRequest) (*entity.Booking, error) {
	service, err := uc.serviceRepo.FindByID(ctx, req.ServiceID)
	if err != nil {
		log.Printf("[BookingUsecase.CreateBooking] find service error: %v", err)
		return nil, err
	}

	if service.Status != "active" {
		return nil, apperror.NewDomainError(apperror.ErrServiceInactive, "SERVICE_INACTIVE", "service is not active")
	}

	scheduleDate, err := parseScheduleDate(req.ScheduleDate)
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_DATE", "invalid schedule date format, expected YYYY-MM-DD")
	}

	subTotal := service.BasePrice
	var bookingAddOns []entity.BookingAddOn

	if len(req.AddOnIDs) > 0 {
		addOnMap := make(map[uint]entity.ServiceAddOn)
		for _, addon := range service.AddOns {
			addOnMap[addon.ID] = addon
		}

		for _, addOnID := range req.AddOnIDs {
			addon, ok := addOnMap[addOnID]
			if !ok {
				return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_ADDON", "add-on does not belong to this service")
			}
			subTotal += addon.Price
			bookingAddOns = append(bookingAddOns, entity.BookingAddOn{
				Name:  addon.Name,
				Price: addon.Price,
			})
		}
	}

	total := subTotal + uc.platformFee

	booking := &entity.Booking{
		CustomerID:    customerID,
		ServiceID:     service.ID,
		ProviderID:    service.ProviderID,
		ScheduleDate:  scheduleDate,
		ScheduleTime:  req.ScheduleTime,
		Address:       req.Address,
		Notes:         req.Notes,
		SubTotal:      subTotal,
		PlatformFee:   uc.platformFee,
		Total:         total,
		PaymentMethod: req.PaymentMethod,
		Status:        "pending",
		BookingAddOns: bookingAddOns,
	}

	if err := uc.bookingRepo.Create(ctx, booking); err != nil {
		log.Printf("[BookingUsecase.CreateBooking] create error: %v", err)
		return nil, err
	}

	return booking, nil
}

func (uc *BookingUsecase) GetBooking(ctx context.Context, userID uint, id uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.GetBooking] find error: %v", err)
		return nil, err
	}

	if booking.CustomerID != userID && booking.ProviderID != userID {
		return nil, apperror.NewDomainError(apperror.ErrNotBookingParticipant, "NOT_PARTICIPANT", "you are not a participant of this booking")
	}

	return booking, nil
}

func (uc *BookingUsecase) GetMyBookings(ctx context.Context, userID uint, role string, filter repository.BookingFilter) ([]*entity.Booking, int64, error) {
	bookings, total, err := uc.bookingRepo.FindByUserID(ctx, userID, role, filter)
	if err != nil {
		log.Printf("[BookingUsecase.GetMyBookings] find error: %v", err)
		return nil, 0, err
	}
	return bookings, total, nil
}

func (uc *BookingUsecase) AcceptBooking(ctx context.Context, providerID uint, id uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.AcceptBooking] find error: %v", err)
		return nil, err
	}

	if booking.ProviderID != providerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the provider can accept this booking")
	}

	if booking.Status != "pending" {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only accept pending bookings")
	}

	if err := uc.bookingRepo.UpdateStatus(ctx, id, "accepted"); err != nil {
		log.Printf("[BookingUsecase.AcceptBooking] update status error: %v", err)
		return nil, err
	}

	booking.Status = "accepted"
	return booking, nil
}

func (uc *BookingUsecase) RejectBooking(ctx context.Context, providerID uint, id uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.RejectBooking] find error: %v", err)
		return nil, err
	}

	if booking.ProviderID != providerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the provider can reject this booking")
	}

	if booking.Status != "pending" {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only reject pending bookings")
	}

	if err := uc.bookingRepo.UpdateStatus(ctx, id, "rejected"); err != nil {
		log.Printf("[BookingUsecase.RejectBooking] update status error: %v", err)
		return nil, err
	}

	booking.Status = "rejected"
	return booking, nil
}

func (uc *BookingUsecase) ConfirmBooking(ctx context.Context, providerID uint, id uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.ConfirmBooking] find error: %v", err)
		return nil, err
	}

	if booking.ProviderID != providerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the provider can confirm this booking")
	}

	if booking.Status != "accepted" {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only confirm accepted bookings")
	}

	if err := uc.bookingRepo.UpdateStatus(ctx, id, "confirmed"); err != nil {
		log.Printf("[BookingUsecase.ConfirmBooking] update status error: %v", err)
		return nil, err
	}

	booking.Status = "confirmed"
	return booking, nil
}

func (uc *BookingUsecase) CompleteBooking(ctx context.Context, customerID uint, id uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.CompleteBooking] find error: %v", err)
		return nil, err
	}

	if booking.CustomerID != customerID {
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "FORBIDDEN", "only the customer can complete this booking")
	}

	if booking.Status != "confirmed" {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only complete confirmed bookings")
	}

	if err := uc.bookingRepo.UpdateStatus(ctx, id, "completed"); err != nil {
		log.Printf("[BookingUsecase.CompleteBooking] update status error: %v", err)
		return nil, err
	}

	booking.Status = "completed"
	return booking, nil
}

func (uc *BookingUsecase) CancelBooking(ctx context.Context, userID uint, id uint) (*entity.Booking, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[BookingUsecase.CancelBooking] find error: %v", err)
		return nil, err
	}

	if booking.CustomerID != userID && booking.ProviderID != userID {
		return nil, apperror.NewDomainError(apperror.ErrNotBookingParticipant, "NOT_PARTICIPANT", "you are not a participant of this booking")
	}

	if booking.Status != "pending" && booking.Status != "accepted" {
		return nil, apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_TRANSITION", "can only cancel pending or accepted bookings")
	}

	if err := uc.bookingRepo.UpdateStatus(ctx, id, "cancelled"); err != nil {
		log.Printf("[BookingUsecase.CancelBooking] update status error: %v", err)
		return nil, err
	}

	booking.Status = "cancelled"
	return booking, nil
}

func parseScheduleDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}

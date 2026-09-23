package usecase

import (
	"context"
	"log"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

type ReviewUsecase struct {
	reviewRepo  repository.ReviewRepository
	bookingRepo repository.BookingRepository
	userRepo    repository.UserRepository
}

func NewReviewUsecase(reviewRepo repository.ReviewRepository, bookingRepo repository.BookingRepository, userRepo repository.UserRepository) *ReviewUsecase {
	return &ReviewUsecase{
		reviewRepo:  reviewRepo,
		bookingRepo: bookingRepo,
		userRepo:    userRepo,
	}
}

func (uc *ReviewUsecase) CreateReview(ctx context.Context, callerID uint, req dto.CreateReviewRequest) (*entity.Review, error) {
	booking, err := uc.bookingRepo.FindByID(ctx, req.BookingID)
	if err != nil {
		log.Printf("[ReviewUsecase.CreateReview] find booking error: %v", err)
		return nil, err
	}

	// Either booking party may review, exactly once per author per booking.
	var authorRole string
	switch callerID {
	case booking.CustomerID:
		authorRole = "customer"
	case booking.ProviderID:
		authorRole = "provider"
	default:
		return nil, apperror.NewDomainError(apperror.ErrForbidden, "NOT_PARTY", "only the booking customer or provider can review")
	}

	if booking.Status != "completed" {
		return nil, apperror.NewDomainError(apperror.ErrBookingNotCompleted, "BOOKING_NOT_COMPLETED", "booking must be completed to leave a review")
	}

	exists, err := uc.reviewRepo.ExistsByBookingAndAuthor(ctx, req.BookingID, callerID)
	if err != nil {
		log.Printf("[ReviewUsecase.CreateReview] exists check error: %v", err)
		return nil, err
	}
	if exists {
		return nil, apperror.NewDomainError(apperror.ErrReviewAlreadyExists, "REVIEW_EXISTS", "you have already reviewed this booking")
	}

	review := &entity.Review{
		BookingID:           req.BookingID,
		ServiceID:           booking.ServiceID,
		CustomerID:          booking.CustomerID,
		ProviderID:          booking.ProviderID,
		AuthorID:            callerID,
		AuthorRole:          authorRole,
		Rating:              req.Rating,
		CommunicationRating: req.CommunicationRating,
		QualityRating:       req.QualityRating,
		ValueRating:         req.ValueRating,
		Comment:             req.Comment,
	}

	if err := uc.reviewRepo.Create(ctx, review); err != nil {
		log.Printf("[ReviewUsecase.CreateReview] create error: %v", err)
		return nil, err
	}

	return review, nil
}

func (uc *ReviewUsecase) GetServiceReviews(ctx context.Context, serviceID uint, page, limit int) ([]*entity.Review, int64, *repository.ServiceReviewStats, error) {
	reviews, total, err := uc.reviewRepo.FindByServiceID(ctx, serviceID, page, limit)
	if err != nil {
		log.Printf("[ReviewUsecase.GetServiceReviews] find error: %v", err)
		return nil, 0, nil, err
	}

	stats, err := uc.reviewRepo.GetServiceStats(ctx, serviceID)
	if err != nil {
		log.Printf("[ReviewUsecase.GetServiceReviews] stats error: %v", err)
		return nil, 0, nil, err
	}

	return reviews, total, stats, nil
}

func (uc *ReviewUsecase) GetProviderReviews(ctx context.Context, providerID uint, page, limit int) ([]*entity.Review, int64, error) {
	reviews, total, err := uc.reviewRepo.FindByProviderID(ctx, providerID, page, limit)
	if err != nil {
		log.Printf("[ReviewUsecase.GetProviderReviews] find error: %v", err)
		return nil, 0, err
	}

	return reviews, total, nil
}

func (uc *ReviewUsecase) GetReceivedReviews(ctx context.Context, userID uint, page, limit int) ([]*entity.Review, int64, error) {
	reviews, total, err := uc.reviewRepo.FindReceivedByUserID(ctx, userID, page, limit)
	if err != nil {
		log.Printf("[ReviewUsecase.GetReceivedReviews] find error: %v", err)
		return nil, 0, err
	}

	return reviews, total, nil
}

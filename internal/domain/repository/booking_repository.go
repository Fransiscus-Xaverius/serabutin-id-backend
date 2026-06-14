package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type BookingRepository interface {
	Create(ctx context.Context, booking *entity.Booking) error
	FindByID(ctx context.Context, id uint) (*entity.Booking, error)
	UpdateStatus(ctx context.Context, id uint, status string) error
	FindByUserID(ctx context.Context, userID uint, role string, filter BookingFilter) ([]*entity.Booking, int64, error)
}

type BookingFilter struct {
	Status string
	Page   int
	Limit  int
}

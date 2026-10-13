package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

type BookingRepository struct {
	db *gorm.DB
}

func NewBookingRepository(db *gorm.DB) *BookingRepository {
	return &BookingRepository{db: db}
}

func (r *BookingRepository) Create(ctx context.Context, booking *entity.Booking) error {
	m := r.toModel(booking)

	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	booking.ID = m.ID
	booking.CreatedAt = m.CreatedAt
	booking.UpdatedAt = m.UpdatedAt

	for i := range m.BookingAddOns {
		if i < len(booking.BookingAddOns) {
			booking.BookingAddOns[i].ID = m.BookingAddOns[i].ID
		}
	}

	return nil
}

func (r *BookingRepository) FindByID(ctx context.Context, id uint) (*entity.Booking, error) {
	var m model.BookingModel

	err := r.db.WithContext(ctx).
		Preload("BookingAddOns").
		First(&m, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntity(&m), nil
}

func (r *BookingRepository) UpdateStatus(ctx context.Context, id uint, status string) error {
	result := r.db.WithContext(ctx).Model(&model.BookingModel{}).Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
	}
	return nil
}

func (r *BookingRepository) UpdateSchedule(ctx context.Context, id uint, scheduleDate time.Time, scheduleTime string, address string) error {
	result := r.db.WithContext(ctx).Model(&model.BookingModel{}).Where("id = ?", id).Updates(map[string]interface{}{
		"schedule_date": scheduleDate,
		"schedule_time": scheduleTime,
		"address":       address,
	})
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
	}
	return nil
}

func (r *BookingRepository) UpdateCancel(ctx context.Context, id uint, cancellationID uint, cancelledBy uint) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":          "cancelled",
		"cancellation_id": cancellationID,
		"cancelled_by":    cancelledBy,
		"cancelled_at":    now,
	}
	result := r.db.WithContext(ctx).Model(&model.BookingModel{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrBookingNotFound, "BOOKING_NOT_FOUND", "booking not found")
	}
	return nil
}

func (r *BookingRepository) FindByUserID(ctx context.Context, userID uint, role string, filter repository.BookingFilter) ([]*entity.Booking, int64, error) {
	var models []model.BookingModel
	var total int64

	query := r.db.WithContext(ctx).Model(&model.BookingModel{})

	if role == "provider" {
		query = query.Where("provider_id = ?", userID)
	} else {
		query = query.Where("customer_id = ?", userID)
	}

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	offset := (filter.Page - 1) * filter.Limit
	err := query.
		Preload("BookingAddOns").
		Order("created_at DESC").
		Offset(offset).
		Limit(filter.Limit).
		Find(&models).Error
	if err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntities(models), total, nil
}

func (r *BookingRepository) FindBookedSlotsByProvider(ctx context.Context, providerID uint, date string) ([]string, error) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_DATE", "invalid date format, expected YYYY-MM-DD")
	}

	var slots []string
	err = r.db.WithContext(ctx).
		Model(&model.BookingModel{}).
		Where("provider_id = ? AND schedule_date = ? AND status NOT IN ?", providerID, parsed, []string{"rejected", "cancelled"}).
		Distinct("schedule_time").
		Pluck("schedule_time", &slots).Error
	if err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return slots, nil
}

func (r *BookingRepository) toModel(booking *entity.Booking) *model.BookingModel {
	m := &model.BookingModel{
		ID:            booking.ID,
		CustomerID:    booking.CustomerID,
		ServiceID:     booking.ServiceID,
		ProviderID:    booking.ProviderID,
		ScheduleDate:  booking.ScheduleDate,
		ScheduleTime:  booking.ScheduleTime,
		Address:       booking.Address,
		Notes:         booking.Notes,
		SubTotal:      booking.SubTotal,
		PlatformFee:   booking.PlatformFee,
		Total:         booking.Total,
		PaymentMethod: booking.PaymentMethod,
		Status:        booking.Status,
	}

	for _, addon := range booking.BookingAddOns {
		m.BookingAddOns = append(m.BookingAddOns, model.BookingAddOnModel{
			ID:        addon.ID,
			BookingID: booking.ID,
			Name:      addon.Name,
			Price:     addon.Price,
		})
	}

	return m
}

func (r *BookingRepository) toEntity(m *model.BookingModel) *entity.Booking {
	b := &entity.Booking{
		ID:             m.ID,
		CustomerID:     m.CustomerID,
		ServiceID:      m.ServiceID,
		ProviderID:     m.ProviderID,
		ScheduleDate:   m.ScheduleDate,
		ScheduleTime:   m.ScheduleTime,
		Address:        m.Address,
		Notes:          m.Notes,
		SubTotal:       m.SubTotal,
		PlatformFee:    m.PlatformFee,
		Total:          m.Total,
		PaymentMethod:  m.PaymentMethod,
		Status:         m.Status,
		CancellationID: m.CancellationID,
		CancelledBy:    m.CancelledBy,
		CancelledAt:    m.CancelledAt,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}

	for _, addon := range m.BookingAddOns {
		b.BookingAddOns = append(b.BookingAddOns, entity.BookingAddOn{
			ID:        addon.ID,
			BookingID: addon.BookingID,
			Name:      addon.Name,
			Price:     addon.Price,
		})
	}

	return b
}

func (r *BookingRepository) toEntities(models []model.BookingModel) []*entity.Booking {
	entities := make([]*entity.Booking, len(models))
	for i := range models {
		entities[i] = r.toEntity(&models[i])
	}
	return entities
}

package gormrepo

import (
	"context"
	"errors"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

type ServiceRepository struct {
	db *gorm.DB
}

func NewServiceRepository(db *gorm.DB) *ServiceRepository {
	return &ServiceRepository{db: db}
}

func (r *ServiceRepository) Create(ctx context.Context, service *entity.Service) error {
	m := r.toModel(service)

	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	service.ID = m.ID
	service.CreatedAt = m.CreatedAt
	service.UpdatedAt = m.UpdatedAt

	for i := range m.Images {
		if i < len(service.Images) {
			service.Images[i].ID = m.Images[i].ID
		}
	}
	for i := range m.AddOns {
		if i < len(service.AddOns) {
			service.AddOns[i].ID = m.AddOns[i].ID
		}
	}

	return nil
}

func (r *ServiceRepository) FindByID(ctx context.Context, id uint) (*entity.Service, error) {
	var m model.ServiceModel

	err := r.db.WithContext(ctx).
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC")
		}).
		Preload("AddOns").
		First(&m, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrServiceNotFound, "SERVICE_NOT_FOUND", "service not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntity(&m), nil
}

func (r *ServiceRepository) Update(ctx context.Context, service *entity.Service) error {
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("service_id = ?", service.ID).Delete(&model.ServiceImageModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("service_id = ?", service.ID).Delete(&model.ServiceAddOnModel{}).Error; err != nil {
			return err
		}

		m := r.toModel(service)
		if err := tx.Save(m).Error; err != nil {
			return err
		}

		return nil
	}); err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return nil
}

func (r *ServiceRepository) List(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	var models []model.ServiceModel
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ServiceModel{}).Where("status = ?", "active")

	if filter.Query != "" {
		query = query.Where("title ILIKE ? OR description ILIKE ?", "%"+filter.Query+"%", "%"+filter.Query+"%")
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.MinPrice > 0 {
		query = query.Where("base_price >= ?", filter.MinPrice)
	}
	if filter.MaxPrice > 0 {
		query = query.Where("base_price <= ?", filter.MaxPrice)
	}
	if filter.City != "" {
		query = query.Joins("JOIN users ON users.id = services.provider_id").Where("users.city = ?", filter.City)
	}
	if filter.ServiceType != "" {
		query = query.Where("service_type = ?", filter.ServiceType)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	switch filter.Sort {
	case "price_asc":
		query = query.Order("base_price ASC")
	case "price_desc":
		query = query.Order("base_price DESC")
	default:
		query = query.Order("created_at DESC")
	}

	offset := (filter.Page - 1) * filter.Limit
	err := query.
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC")
		}).
		Preload("AddOns").
		Offset(offset).
		Limit(filter.Limit).
		Find(&models).Error
	if err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntities(models), total, nil
}

func (r *ServiceRepository) FindByProviderID(ctx context.Context, providerID uint, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	var models []model.ServiceModel
	var total int64

	query := r.db.WithContext(ctx).Model(&model.ServiceModel{}).Where("provider_id = ?", providerID)

	if filter.Query != "" {
		query = query.Where("title ILIKE ? OR description ILIKE ?", "%"+filter.Query+"%", "%"+filter.Query+"%")
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	offset := (filter.Page - 1) * filter.Limit
	err := query.
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC")
		}).
		Preload("AddOns").
		Order("created_at DESC").
		Offset(offset).
		Limit(filter.Limit).
		Find(&models).Error
	if err != nil {
		return nil, 0, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}

	return r.toEntities(models), total, nil
}

func (r *ServiceRepository) SetStatus(ctx context.Context, id uint, status string) error {
	result := r.db.WithContext(ctx).Model(&model.ServiceModel{}).Where("id = ?", id).Update("status", status)
	if result.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", result.Error.Error())
	}
	if result.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrServiceNotFound, "SERVICE_NOT_FOUND", "service not found")
	}
	return nil
}

func (r *ServiceRepository) toModel(service *entity.Service) *model.ServiceModel {
	m := &model.ServiceModel{
		ID:              service.ID,
		ProviderID:      service.ProviderID,
		Title:           service.Title,
		Category:        service.Category,
		Description:     service.Description,
		UnitType:        service.UnitType,
		BasePrice:       service.BasePrice,
		DurationMinutes: service.DurationMinutes,
		ServiceAreas:    service.ServiceAreas,
		ServiceType:     service.ServiceType,
		Status:          service.Status,
	}

	for _, img := range service.Images {
		m.Images = append(m.Images, model.ServiceImageModel{
			ID:        img.ID,
			ServiceID: service.ID,
			URL:       img.URL,
			SortOrder: img.SortOrder,
		})
	}

	for _, addon := range service.AddOns {
		m.AddOns = append(m.AddOns, model.ServiceAddOnModel{
			ID:        addon.ID,
			ServiceID: service.ID,
			Name:      addon.Name,
			Price:     addon.Price,
		})
	}

	return m
}

func (r *ServiceRepository) toEntity(m *model.ServiceModel) *entity.Service {
	s := &entity.Service{
		ID:              m.ID,
		ProviderID:      m.ProviderID,
		Title:           m.Title,
		Category:        m.Category,
		Description:     m.Description,
		UnitType:        m.UnitType,
		BasePrice:       m.BasePrice,
		DurationMinutes: m.DurationMinutes,
		ServiceAreas:    m.ServiceAreas,
		ServiceType:     m.ServiceType,
		Status:          m.Status,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}

	for _, img := range m.Images {
		s.Images = append(s.Images, entity.ServiceImage{
			ID:        img.ID,
			ServiceID: img.ServiceID,
			URL:       img.URL,
			SortOrder: img.SortOrder,
		})
	}

	for _, addon := range m.AddOns {
		s.AddOns = append(s.AddOns, entity.ServiceAddOn{
			ID:        addon.ID,
			ServiceID: addon.ServiceID,
			Name:      addon.Name,
			Price:     addon.Price,
		})
	}

	return s
}

func (r *ServiceRepository) toEntities(models []model.ServiceModel) []*entity.Service {
	entities := make([]*entity.Service, len(models))
	for i := range models {
		entities[i] = r.toEntity(&models[i])
	}
	return entities
}

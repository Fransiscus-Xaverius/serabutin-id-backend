package usecase

import (
	"context"
	"log"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

var validCategories = map[string]bool{
	"cleaning":  true,
	"handyman":  true,
	"massage":   true,
	"gardening": true,
	"ac_service": true,
	"moving":    true,
	"laundry":   true,
	"shopping":  true,
	"errands":   true,
	"other":     true,
}

type ServiceUsecase struct {
	serviceRepo repository.ServiceRepository
	userRepo    repository.UserRepository
}

func NewServiceUsecase(serviceRepo repository.ServiceRepository, userRepo repository.UserRepository) *ServiceUsecase {
	return &ServiceUsecase{
		serviceRepo: serviceRepo,
		userRepo:    userRepo,
	}
}

func (uc *ServiceUsecase) CreateService(ctx context.Context, providerID uint, req dto.CreateServiceRequest) (*entity.Service, error) {
	if !validCategories[req.Category] {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_CATEGORY", "invalid service category")
	}

	status := req.Status
	if status == "" {
		status = "draft"
	}

	service := &entity.Service{
		ProviderID:      providerID,
		Title:           req.Title,
		Category:        req.Category,
		Description:     req.Description,
		UnitType:        req.UnitType,
		BasePrice:       req.BasePrice,
		DurationMinutes: req.DurationMinutes,
		ServiceAreas:    req.ServiceAreas,
		Status:          status,
	}

	for _, img := range req.Images {
		service.Images = append(service.Images, entity.ServiceImage{
			URL:       img.URL,
			SortOrder: img.SortOrder,
		})
	}

	for _, addon := range req.AddOns {
		service.AddOns = append(service.AddOns, entity.ServiceAddOn{
			Name:  addon.Name,
			Price: addon.Price,
		})
	}

	if err := uc.serviceRepo.Create(ctx, service); err != nil {
		log.Printf("[ServiceUsecase.CreateService] create error: %v", err)
		return nil, err
	}

	return service, nil
}

func (uc *ServiceUsecase) GetService(ctx context.Context, id uint) (*entity.Service, error) {
	service, err := uc.serviceRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[ServiceUsecase.GetService] find error: %v", err)
		return nil, err
	}
	return service, nil
}

func (uc *ServiceUsecase) UpdateService(ctx context.Context, providerID uint, id uint, req dto.UpdateServiceRequest) (*entity.Service, error) {
	service, err := uc.serviceRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[ServiceUsecase.UpdateService] find error: %v", err)
		return nil, err
	}

	if service.ProviderID != providerID {
		return nil, apperror.NewDomainError(apperror.ErrNotServiceOwner, "NOT_OWNER", "you are not the service owner")
	}

	if req.Title != "" {
		service.Title = req.Title
	}
	if req.Category != "" {
		if !validCategories[req.Category] {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_CATEGORY", "invalid service category")
		}
		service.Category = req.Category
	}
	if req.Description != "" {
		service.Description = req.Description
	}
	if req.UnitType != "" {
		service.UnitType = req.UnitType
	}
	if req.BasePrice > 0 {
		service.BasePrice = req.BasePrice
	}
	if req.DurationMinutes > 0 {
		service.DurationMinutes = req.DurationMinutes
	}
	if req.ServiceAreas != "" {
		service.ServiceAreas = req.ServiceAreas
	}
	if req.Status != "" {
		service.Status = req.Status
	}
	if req.Images != nil {
		service.Images = nil
		for _, img := range req.Images {
			service.Images = append(service.Images, entity.ServiceImage{
				URL:       img.URL,
				SortOrder: img.SortOrder,
			})
		}
	}
	if req.AddOns != nil {
		service.AddOns = nil
		for _, addon := range req.AddOns {
			service.AddOns = append(service.AddOns, entity.ServiceAddOn{
				Name:  addon.Name,
				Price: addon.Price,
			})
		}
	}

	if err := uc.serviceRepo.Update(ctx, service); err != nil {
		log.Printf("[ServiceUsecase.UpdateService] update error: %v", err)
		return nil, err
	}

	return service, nil
}

func (uc *ServiceUsecase) ListServices(ctx context.Context, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	services, total, err := uc.serviceRepo.List(ctx, filter)
	if err != nil {
		log.Printf("[ServiceUsecase.ListServices] list error: %v", err)
		return nil, 0, err
	}
	return services, total, nil
}

func (uc *ServiceUsecase) GetMyServices(ctx context.Context, providerID uint, filter repository.ServiceFilter) ([]*entity.Service, int64, error) {
	services, total, err := uc.serviceRepo.FindByProviderID(ctx, providerID, filter)
	if err != nil {
		log.Printf("[ServiceUsecase.GetMyServices] find error: %v", err)
		return nil, 0, err
	}
	return services, total, nil
}

func (uc *ServiceUsecase) SetServiceStatus(ctx context.Context, providerID uint, id uint, status string) error {
	service, err := uc.serviceRepo.FindByID(ctx, id)
	if err != nil {
		log.Printf("[ServiceUsecase.SetServiceStatus] find error: %v", err)
		return err
	}

	if service.ProviderID != providerID {
		return apperror.NewDomainError(apperror.ErrNotServiceOwner, "NOT_OWNER", "you are not the service owner")
	}

	if err := uc.serviceRepo.SetStatus(ctx, id, status); err != nil {
		log.Printf("[ServiceUsecase.SetServiceStatus] set status error: %v", err)
		return err
	}

	return nil
}

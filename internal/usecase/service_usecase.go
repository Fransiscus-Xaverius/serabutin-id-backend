package usecase

import (
	"context"
	"log"
	"sync"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

var validCategories = map[string]bool{
	"cleaning":   true,
	"handyman":   true,
	"massage":    true,
	"gardening":  true,
	"ac_service": true,
	"moving":     true,
	"laundry":    true,
	"shopping":   true,
	"errands":    true,
	"other":      true,
}

type ServiceUsecase struct {
	serviceRepo repository.ServiceRepository
	userRepo    repository.UserRepository
	reviewRepo  repository.ReviewRepository
	// categoryRepo is attached via SetCategoryRepository (ticket 10). When
	// set, service categories validate against the active taxonomy table
	// (422 UNKNOWN_CATEGORY); when nil, the legacy allowlist applies.
	categoryRepo repository.CategoryRepository
}

func NewServiceUsecase(serviceRepo repository.ServiceRepository, userRepo repository.UserRepository, reviewRepo repository.ReviewRepository) *ServiceUsecase {
	return &ServiceUsecase{
		serviceRepo: serviceRepo,
		userRepo:    userRepo,
		reviewRepo:  reviewRepo,
	}
}

// SetCategoryRepository attaches the taxonomy store so service create/update
// validates `category` against active category rows. Kept as a setter
// (mirroring the notifier pattern) so the established constructor shape —
// relied on by existing usecase tests — stays stable.
func (uc *ServiceUsecase) SetCategoryRepository(r repository.CategoryRepository) {
	uc.categoryRepo = r
}

// validateCategory enforces the taxonomy: with a repository attached the
// slug must name an active category row (422 UNKNOWN_CATEGORY, after the
// startup backfill seeds live values); without one the legacy allowlist
// applies (existing unit tests exercise this path).
func (uc *ServiceUsecase) validateCategory(ctx context.Context, slug string) error {
	if uc.categoryRepo != nil {
		ok, err := uc.categoryRepo.ExistsActive(ctx, slug)
		if err != nil {
			log.Printf("[ServiceUsecase.validateCategory] taxonomy lookup error: %v", err)
			return err
		}
		if !ok {
			return apperror.NewDomainError(apperror.ErrValidation, "UNKNOWN_CATEGORY", "unknown service category")
		}
		return nil
	}
	if !validCategories[slug] {
		return apperror.NewDomainError(apperror.ErrValidation, "INVALID_CATEGORY", "invalid service category")
	}
	return nil
}

func (uc *ServiceUsecase) CreateService(ctx context.Context, providerID uint, req dto.CreateServiceRequest) (*entity.Service, error) {
	if err := uc.validateCategory(ctx, req.Category); err != nil {
		return nil, err
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
		BudgetMin:       req.BudgetMin,
		BudgetMax:       req.BudgetMax,
		AutoAcceptPrice: req.AutoAcceptPrice,
		DurationMinutes: req.DurationMinutes,
		ServiceAreas:    req.ServiceAreas,
		ServiceType:     req.ServiceType,
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
		if err := uc.validateCategory(ctx, req.Category); err != nil {
			return nil, err
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

func (uc *ServiceUsecase) GetReviewStats(ctx context.Context, serviceID uint) *repository.ServiceReviewStats {
	stats, err := uc.reviewRepo.GetServiceStats(ctx, serviceID)
	if err != nil {
		log.Printf("[ServiceUsecase.GetReviewStats] stats error: %v", err)
		return &repository.ServiceReviewStats{}
	}
	return stats
}

func (uc *ServiceUsecase) GetReviewStatsMap(ctx context.Context, serviceIDs []uint) map[uint]*repository.ServiceReviewStats {
	result := make(map[uint]*repository.ServiceReviewStats)
	if len(serviceIDs) == 0 {
		return result
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, id := range serviceIDs {
		wg.Add(1)
		go func(sid uint) {
			defer wg.Done()
			stats, err := uc.reviewRepo.GetServiceStats(ctx, sid)
			if err != nil {
				log.Printf("[ServiceUsecase.GetReviewStatsMap] stats error for service %d: %v", sid, err)
				stats = &repository.ServiceReviewStats{}
			}
			mu.Lock()
			result[sid] = stats
			mu.Unlock()
		}(id)
	}

	wg.Wait()
	return result
}

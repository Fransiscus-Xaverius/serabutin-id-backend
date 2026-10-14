package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type ServiceRepository interface {
	Create(ctx context.Context, service *entity.Service) error
	FindByID(ctx context.Context, id uint) (*entity.Service, error)
	Update(ctx context.Context, service *entity.Service) error
	List(ctx context.Context, filter ServiceFilter) ([]*entity.Service, int64, error)
	FindByProviderID(ctx context.Context, providerID uint, filter ServiceFilter) ([]*entity.Service, int64, error)
	SetStatus(ctx context.Context, id uint, status string) error
	// ListForAdmin bypasses the public status='active' gate. Status carries
	// an optional comma-separated allowlist (e.g. "draft,pending"); empty
	// means all statuses.
	ListForAdmin(ctx context.Context, filter ServiceFilter) ([]*entity.Service, int64, error)
}

type ServiceFilter struct {
	Query       string
	Category    string
	Status      string
	City        string
	MinPrice    int64
	MaxPrice    int64
	MinPriceSet bool
	MaxPriceSet bool
	Sort        string
	Page        int
	Limit       int
	ServiceType string
	PriceRanges []PriceRange
	MinRating   float64
}

type PriceRange struct {
	Min int64
	Max *int64
}

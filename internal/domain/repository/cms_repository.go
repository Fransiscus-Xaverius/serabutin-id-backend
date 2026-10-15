package repository

import (
	"context"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

// BannerRepository owns banner persistence for the admin CMS panel and the
// public storefront read. Ticket 10.
type BannerRepository interface {
	Create(ctx context.Context, b *entity.Banner) error
	FindByID(ctx context.Context, id uint) (*entity.Banner, error)
	// ListForAdmin returns every banner, newest first.
	ListForAdmin(ctx context.Context) ([]*entity.Banner, error)
	// ListPublic returns active banners in their schedule window for a
	// placement, ordered by sort_order, id.
	ListPublic(ctx context.Context, placement string, now time.Time) ([]*entity.Banner, error)
	Update(ctx context.Context, b *entity.Banner) error
	Delete(ctx context.Context, id uint) error
}

// CategoryRepository owns the service taxonomy table. Ticket 10.
type CategoryRepository interface {
	Create(ctx context.Context, c *entity.Category) error
	FindByID(ctx context.Context, id uint) (*entity.Category, error)
	FindBySlug(ctx context.Context, slug string) (*entity.Category, error)
	// ListForAdmin returns every category ordered by sort_order, id.
	ListForAdmin(ctx context.Context) ([]*entity.Category, error)
	// ListPublic returns active categories ordered by sort_order, id.
	ListPublic(ctx context.Context) ([]*entity.Category, error)
	// ExistsActive reports whether slug names an active taxonomy row.
	// Service create/update enforces this (422 UNKNOWN_CATEGORY).
	ExistsActive(ctx context.Context, slug string) (bool, error)
	Update(ctx context.Context, c *entity.Category) error
	Delete(ctx context.Context, id uint) error
	// DistinctServiceCategories returns the distinct non-empty
	// services.category values — the backfill source for the taxonomy.
	DistinctServiceCategories(ctx context.Context) ([]string, error)
	// BackfillDistinct inserts one active row per slug, skipping values
	// already seeded. Returns the number of rows inserted.
	BackfillDistinct(ctx context.Context, slugs []string) (int64, error)
}

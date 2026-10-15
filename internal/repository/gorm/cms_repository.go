package gormrepo

import (
	"context"
	"strings"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
)

var (
	_ repository.BannerRepository   = (*BannerRepository)(nil)
	_ repository.CategoryRepository = (*CategoryRepository)(nil)
)

// BannerRepository is the GORM banner store for the admin CMS panel and the
// public storefront read. Ticket 10.
type BannerRepository struct {
	db *gorm.DB
}

func NewBannerRepository(db *gorm.DB) *BannerRepository {
	return &BannerRepository{db: db}
}

func toBannerEntity(m *model.BannerModel) *entity.Banner {
	return &entity.Banner{
		ID:        m.ID,
		Title:     m.Title,
		ImageURL:  m.ImageURL,
		LinkURL:   m.LinkURL,
		Placement: m.Placement,
		SortOrder: m.SortOrder,
		Active:    m.Active,
		StartsAt:  m.StartsAt,
		EndsAt:    m.EndsAt,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func (r *BannerRepository) Create(ctx context.Context, b *entity.Banner) error {
	m := &model.BannerModel{
		Title:     b.Title,
		ImageURL:  b.ImageURL,
		LinkURL:   b.LinkURL,
		Placement: b.Placement,
		SortOrder: b.SortOrder,
		Active:    b.Active,
		StartsAt:  b.StartsAt,
		EndsAt:    b.EndsAt,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	b.ID = m.ID
	b.CreatedAt = m.CreatedAt
	b.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *BannerRepository) FindByID(ctx context.Context, id uint) (*entity.Banner, error) {
	var m model.BannerModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "BANNER_NOT_FOUND", "banner not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return toBannerEntity(&m), nil
}

// ListForAdmin returns every banner, newest first (drafts and expired rows
// stay inspectable).
func (r *BannerRepository) ListForAdmin(ctx context.Context) ([]*entity.Banner, error) {
	var models []model.BannerModel
	if err := r.db.WithContext(ctx).Order("id DESC").Find(&models).Error; err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.Banner, len(models))
	for i := range models {
		out[i] = toBannerEntity(&models[i])
	}
	return out, nil
}

// ListPublic enforces the schedule gate: active AND (starts_at IS NULL OR
// <= now) AND (ends_at IS NULL OR > now), ordered sort_order, id.
func (r *BannerRepository) ListPublic(ctx context.Context, placement string, now time.Time) ([]*entity.Banner, error) {
	var models []model.BannerModel
	q := r.db.WithContext(ctx).
		Where("active = ?", true).
		Where(r.db.Where("starts_at IS NULL").Or("starts_at <= ?", now)).
		Where(r.db.Where("ends_at IS NULL").Or("ends_at > ?", now)).
		Order("sort_order ASC, id ASC")
	if strings.TrimSpace(placement) != "" {
		q = q.Where("placement = ?", strings.TrimSpace(placement))
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.Banner, len(models))
	for i := range models {
		out[i] = toBannerEntity(&models[i])
	}
	return out, nil
}

func (r *BannerRepository) Update(ctx context.Context, b *entity.Banner) error {
	res := r.db.WithContext(ctx).Model(&model.BannerModel{}).Where("id = ?", b.ID).Updates(map[string]interface{}{
		"title":      b.Title,
		"image_url":  b.ImageURL,
		"link_url":   b.LinkURL,
		"placement":  b.Placement,
		"sort_order": b.SortOrder,
		"active":     b.Active,
		"starts_at":  b.StartsAt,
		"ends_at":    b.EndsAt,
		"updated_at": time.Now(),
	})
	if res.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "BANNER_NOT_FOUND", "banner not found")
	}
	return nil
}

func (r *BannerRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.BannerModel{}, id)
	if res.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "BANNER_NOT_FOUND", "banner not found")
	}
	return nil
}

// CategoryRepository is the GORM taxonomy store. Ticket 10.
type CategoryRepository struct {
	db *gorm.DB
}

func NewCategoryRepository(db *gorm.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

func toCategoryEntity(m *model.CategoryModel) *entity.Category {
	return &entity.Category{
		ID:        m.ID,
		Slug:      m.Slug,
		Name:      m.Name,
		Icon:      m.Icon,
		SortOrder: m.SortOrder,
		Active:    m.Active,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func isDuplicateKey(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")
}

func (r *CategoryRepository) Create(ctx context.Context, c *entity.Category) error {
	m := &model.CategoryModel{
		Slug:      c.Slug,
		Name:      c.Name,
		Icon:      c.Icon,
		SortOrder: c.SortOrder,
		Active:    c.Active,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if isDuplicateKey(err) {
			return apperror.NewDomainError(apperror.ErrAlreadyExists, "CATEGORY_EXISTS", "category slug already exists")
		}
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	c.ID = m.ID
	c.CreatedAt = m.CreatedAt
	c.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *CategoryRepository) FindByID(ctx context.Context, id uint) (*entity.Category, error) {
	var m model.CategoryModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return toCategoryEntity(&m), nil
}

func (r *CategoryRepository) FindBySlug(ctx context.Context, slug string) (*entity.Category, error) {
	var m model.CategoryModel
	if err := r.db.WithContext(ctx).Where("slug = ?", slug).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return toCategoryEntity(&m), nil
}

// ListForAdmin returns every category ordered by sort_order, id (inactive
// rows included).
func (r *CategoryRepository) ListForAdmin(ctx context.Context) ([]*entity.Category, error) {
	var models []model.CategoryModel
	if err := r.db.WithContext(ctx).Order("sort_order ASC, id ASC").Find(&models).Error; err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.Category, len(models))
	for i := range models {
		out[i] = toCategoryEntity(&models[i])
	}
	return out, nil
}

// ListPublic returns active categories ordered by sort_order, id.
func (r *CategoryRepository) ListPublic(ctx context.Context) ([]*entity.Category, error) {
	var models []model.CategoryModel
	if err := r.db.WithContext(ctx).Where("active = ?", true).Order("sort_order ASC, id ASC").Find(&models).Error; err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	out := make([]*entity.Category, len(models))
	for i := range models {
		out[i] = toCategoryEntity(&models[i])
	}
	return out, nil
}

// ExistsActive reports whether slug names an active taxonomy row. Service
// create/update enforces this (422 UNKNOWN_CATEGORY).
func (r *CategoryRepository) ExistsActive(ctx context.Context, slug string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.CategoryModel{}).
		Where("slug = ? AND active = ?", strings.TrimSpace(slug), true).
		Count(&count).Error; err != nil {
		return false, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return count > 0, nil
}

func (r *CategoryRepository) Update(ctx context.Context, c *entity.Category) error {
	res := r.db.WithContext(ctx).Model(&model.CategoryModel{}).Where("id = ?", c.ID).Updates(map[string]interface{}{
		"slug":       c.Slug,
		"name":       c.Name,
		"icon":       c.Icon,
		"sort_order": c.SortOrder,
		"active":     c.Active,
		"updated_at": time.Now(),
	})
	if res.Error != nil {
		if isDuplicateKey(res.Error) {
			return apperror.NewDomainError(apperror.ErrAlreadyExists, "CATEGORY_EXISTS", "category slug already exists")
		}
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
	}
	return nil
}

func (r *CategoryRepository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.CategoryModel{}, id)
	if res.Error != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
	}
	return nil
}

// BackfillDistinct inserts one active row per distinct non-empty slug,
// skipping values already seeded. Names default to the slug when no
// friendlier label is known; callers pass the canonical taxonomy first so
// ordering stays stable.
func (r *CategoryRepository) BackfillDistinct(ctx context.Context, slugs []string) (int64, error) {
	var inserted int64
	order := 0
	for _, raw := range slugs {
		slug := strings.TrimSpace(raw)
		if slug == "" {
			continue
		}
		var count int64
		if err := r.db.WithContext(ctx).Model(&model.CategoryModel{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
			return inserted, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
		}
		if count > 0 {
			continue
		}
		m := &model.CategoryModel{Slug: slug, Name: slug, SortOrder: order, Active: true}
		if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
			if isDuplicateKey(err) {
				continue
			}
			return inserted, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
		}
		inserted++
		order++
	}
	return inserted, nil
}

// DistinctServiceCategories returns the distinct non-empty services.category
// values — the backfill source for the taxonomy table.
func (r *CategoryRepository) DistinctServiceCategories(ctx context.Context) ([]string, error) {
	var values []string
	if err := r.db.WithContext(ctx).Model(&model.ServiceModel{}).
		Distinct().Where("category <> ''").Pluck("category", &values).Error; err != nil {
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return values, nil
}

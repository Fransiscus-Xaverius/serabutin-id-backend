package usecase

import (
	"context"
	"errors"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

var categorySlugPattern = regexp.MustCompile(`^[a-z0-9-]{1,50}$`)

// seedCategories is the canonical taxonomy backfilled on startup (slug ->
// display name), before distinct live service values are seeded. Display
// names match the storefront labels.
var seedCategories = []entity.Category{
	{Slug: "cleaning", Name: "Cleaning Rumah", Icon: "Sparkles"},
	{Slug: "handyman", Name: "Tukang & Perbaikan", Icon: "Wrench"},
	{Slug: "massage", Name: "Pijat & Wellness", Icon: "HeartPulse"},
	{Slug: "gardening", Name: "Kebun & Taman", Icon: "Leaf"},
	{Slug: "ac_service", Name: "Service AC", Icon: "Snowflake"},
	{Slug: "moving", Name: "Pindahan & Angkut", Icon: "Truck"},
	{Slug: "laundry", Name: "Laundry & Setrika", Icon: "Shirt"},
	{Slug: "shopping", Name: "Belanja & Titip", Icon: "ShoppingBag"},
	{Slug: "errands", Name: "Errands", Icon: "Package"},
	{Slug: "other", Name: "Lainnya", Icon: "Grid3x3"},
}

// SlugifyCategory derives a taxonomy slug from free text: lowercase,
// spaces/underscores collapse to one hyphen, everything outside [a-z0-9-]
// is dropped.
func SlugifyCategory(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevHyphen := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case r == ' ' || r == '_' || r == '-':
			if !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// AdminCMSUsecase owns banner CRUD, category CRUD, the two public reads, and
// the service-taxonomy check. Every admin mutation is audit-logged by
// middleware.AdminAuditLogger, so handlers enforce the mandatory `reason`
// and this layer never writes audit rows directly.
type AdminCMSUsecase struct {
	banners    repository.BannerRepository
	categories repository.CategoryRepository
}

func NewAdminCMSUsecase(banners repository.BannerRepository, categories repository.CategoryRepository) *AdminCMSUsecase {
	return &AdminCMSUsecase{banners: banners, categories: categories}
}

// --- banners ---

func (uc *AdminCMSUsecase) CreateBanner(ctx context.Context, title, imageURL, linkURL, placement string, sortOrder int, active bool, startsAt, endsAt *time.Time) (*entity.Banner, error) {
	title = strings.TrimSpace(title)
	imageURL = strings.TrimSpace(imageURL)
	if title == "" {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "title is required")
	}
	if len(title) > 255 {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "title must be at most 255 characters")
	}
	if imageURL == "" {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "image_url is required")
	}
	if len(imageURL) > 500 {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "image_url must be at most 500 characters")
	}
	placement = strings.TrimSpace(placement)
	if placement == "" {
		placement = "home"
	}
	if len(placement) > 30 {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "placement must be at most 30 characters")
	}
	if len(strings.TrimSpace(linkURL)) > 500 {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "link_url must be at most 500 characters")
	}
	if err := checkScheduleWindow(startsAt, endsAt); err != nil {
		return nil, err
	}
	b := &entity.Banner{
		Title:     title,
		ImageURL:  imageURL,
		LinkURL:   strings.TrimSpace(linkURL),
		Placement: placement,
		SortOrder: sortOrder,
		Active:    active,
		StartsAt:  startsAt,
		EndsAt:    endsAt,
	}
	if err := uc.banners.Create(ctx, b); err != nil {
		log.Printf("[AdminCMSUsecase.CreateBanner] create error: %v", err)
		return nil, err
	}
	return b, nil
}

func checkScheduleWindow(startsAt, endsAt *time.Time) error {
	if startsAt != nil && endsAt != nil && !endsAt.After(*startsAt) {
		return apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "ends_at must be after starts_at")
	}
	return nil
}

func (uc *AdminCMSUsecase) ListBannersForAdmin(ctx context.Context) ([]*entity.Banner, error) {
	rows, err := uc.banners.ListForAdmin(ctx)
	if err != nil {
		log.Printf("[AdminCMSUsecase.ListBannersForAdmin] list error: %v", err)
		return nil, err
	}
	return rows, nil
}

func (uc *AdminCMSUsecase) ListPublicBanners(ctx context.Context, placement string) ([]*entity.Banner, error) {
	if strings.TrimSpace(placement) == "" {
		placement = "home"
	}
	rows, err := uc.banners.ListPublic(ctx, placement, time.Now())
	if err != nil {
		log.Printf("[AdminCMSUsecase.ListPublicBanners] list error: %v", err)
		return nil, err
	}
	return rows, nil
}

// BannerPatch is a partial banner update. Nil pointers keep the current
// value; StartsAt/EndsAt take RFC 3339 strings where "" clears the bound.
type BannerPatch struct {
	Title     *string
	ImageURL  *string
	LinkURL   *string
	Placement *string
	SortOrder *int
	Active    *bool
	StartsAt  *string
	EndsAt    *string
}

func parseScheduleBound(raw *string) (*time.Time, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	if strings.TrimSpace(*raw) == "" {
		return nil, true, nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*raw))
	if err != nil {
		return nil, false, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "schedule bounds must be RFC 3339")
	}
	return &parsed, true, nil
}

func (uc *AdminCMSUsecase) UpdateBanner(ctx context.Context, id uint, patch BannerPatch) (*entity.Banner, error) {
	b, err := uc.banners.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "title is required")
		}
		if len(title) > 255 {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "title must be at most 255 characters")
		}
		b.Title = title
	}
	if patch.ImageURL != nil {
		imageURL := strings.TrimSpace(*patch.ImageURL)
		if imageURL == "" {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "image_url is required")
		}
		if len(imageURL) > 500 {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "image_url must be at most 500 characters")
		}
		b.ImageURL = imageURL
	}
	if patch.LinkURL != nil {
		if len(strings.TrimSpace(*patch.LinkURL)) > 500 {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "link_url must be at most 500 characters")
		}
		b.LinkURL = strings.TrimSpace(*patch.LinkURL)
	}
	if patch.Placement != nil {
		placement := strings.TrimSpace(*patch.Placement)
		if placement == "" {
			placement = "home"
		}
		if len(placement) > 30 {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "placement must be at most 30 characters")
		}
		b.Placement = placement
	}
	if patch.SortOrder != nil {
		b.SortOrder = *patch.SortOrder
	}
	if patch.Active != nil {
		b.Active = *patch.Active
	}
	if startsAt, set, err := parseScheduleBound(patch.StartsAt); err != nil {
		return nil, err
	} else if set {
		b.StartsAt = startsAt
	}
	if endsAt, set, err := parseScheduleBound(patch.EndsAt); err != nil {
		return nil, err
	} else if set {
		b.EndsAt = endsAt
	}
	if err := checkScheduleWindow(b.StartsAt, b.EndsAt); err != nil {
		return nil, err
	}
	if err := uc.banners.Update(ctx, b); err != nil {
		log.Printf("[AdminCMSUsecase.UpdateBanner] update error: %v", err)
		return nil, err
	}
	return b, nil
}

func (uc *AdminCMSUsecase) DeleteBanner(ctx context.Context, id uint) error {
	if err := uc.banners.Delete(ctx, id); err != nil {
		log.Printf("[AdminCMSUsecase.DeleteBanner] delete error: %v", err)
		return err
	}
	return nil
}

// --- categories ---

func validateCategorySlug(slug string) error {
	if !categorySlugPattern.MatchString(slug) {
		return apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "slug must be 1-50 chars of lowercase letters, digits, or hyphens")
	}
	return nil
}

func (uc *AdminCMSUsecase) CreateCategory(ctx context.Context, slug, name, icon string, sortOrder int, active bool) (*entity.Category, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "name is required")
	}
	if len(name) > 100 {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "name must be at most 100 characters")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = SlugifyCategory(name)
	}
	if err := validateCategorySlug(slug); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(icon)) > 50 {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "icon must be at most 50 characters")
	}
	c := &entity.Category{
		Slug:      slug,
		Name:      name,
		Icon:      strings.TrimSpace(icon),
		SortOrder: sortOrder,
		Active:    active,
	}
	if err := uc.categories.Create(ctx, c); err != nil {
		log.Printf("[AdminCMSUsecase.CreateCategory] create error: %v", err)
		return nil, err
	}
	return c, nil
}

func (uc *AdminCMSUsecase) ListCategoriesForAdmin(ctx context.Context) ([]*entity.Category, error) {
	rows, err := uc.categories.ListForAdmin(ctx)
	if err != nil {
		log.Printf("[AdminCMSUsecase.ListCategoriesForAdmin] list error: %v", err)
		return nil, err
	}
	return rows, nil
}

func (uc *AdminCMSUsecase) ListPublicCategories(ctx context.Context) ([]*entity.Category, error) {
	rows, err := uc.categories.ListPublic(ctx)
	if err != nil {
		log.Printf("[AdminCMSUsecase.ListPublicCategories] list error: %v", err)
		return nil, err
	}
	return rows, nil
}

// CategoryPatch is a partial category update. Nil pointers keep the current
// value. Deactivating a row with live services is allowed — services keep
// their old value; providers just can't pick the slug anymore.
type CategoryPatch struct {
	Slug      *string
	Name      *string
	Icon      *string
	SortOrder *int
	Active    *bool
}

func (uc *AdminCMSUsecase) UpdateCategory(ctx context.Context, id uint, patch CategoryPatch) (*entity.Category, error) {
	c, err := uc.categories.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if patch.Slug != nil {
		slug := strings.TrimSpace(*patch.Slug)
		if err := validateCategorySlug(slug); err != nil {
			return nil, err
		}
		c.Slug = slug
	}
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "name is required")
		}
		if len(name) > 100 {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "name must be at most 100 characters")
		}
		c.Name = name
	}
	if patch.Icon != nil {
		if len(strings.TrimSpace(*patch.Icon)) > 50 {
			return nil, apperror.NewDomainError(apperror.ErrValidation, "VALIDATION_ERROR", "icon must be at most 50 characters")
		}
		c.Icon = strings.TrimSpace(*patch.Icon)
	}
	if patch.SortOrder != nil {
		c.SortOrder = *patch.SortOrder
	}
	if patch.Active != nil {
		c.Active = *patch.Active
	}
	if err := uc.categories.Update(ctx, c); err != nil {
		log.Printf("[AdminCMSUsecase.UpdateCategory] update error: %v", err)
		return nil, err
	}
	return c, nil
}

func (uc *AdminCMSUsecase) DeleteCategory(ctx context.Context, id uint) error {
	if err := uc.categories.Delete(ctx, id); err != nil {
		log.Printf("[AdminCMSUsecase.DeleteCategory] delete error: %v", err)
		return err
	}
	return nil
}

// ValidateServiceCategory enforces the taxonomy on service create/update:
// the slug must name an active category row, else 422 UNKNOWN_CATEGORY.
func (uc *AdminCMSUsecase) ValidateServiceCategory(ctx context.Context, slug string) error {
	ok, err := uc.categories.ExistsActive(ctx, strings.TrimSpace(slug))
	if err != nil {
		log.Printf("[AdminCMSUsecase.ValidateServiceCategory] lookup error: %v", err)
		return err
	}
	if !ok {
		return apperror.NewDomainError(apperror.ErrValidation, "UNKNOWN_CATEGORY", "unknown service category")
	}
	return nil
}

// EnsureSeedCategories inserts the canonical taxonomy, then backfills one
// row per distinct live service category (slugified), so enforcing the
// taxonomy immediately after never breaks existing listings. Idempotent —
// existing slugs are skipped. Returns total rows inserted.
func (uc *AdminCMSUsecase) EnsureSeedCategories(ctx context.Context) (int64, error) {
	var inserted int64
	for i := range seedCategories {
		seed := seedCategories[i]
		seed.Active = true
		seed.SortOrder = i
		if err := uc.categories.Create(ctx, &seed); err != nil {
			var domainErr *apperror.DomainError
			if errors.As(err, &domainErr) && domainErr.Code == "CATEGORY_EXISTS" {
				continue
			}
			log.Printf("[AdminCMSUsecase.EnsureSeedCategories] seed error: %v", err)
			return inserted, err
		}
		inserted++
	}
	distinct, err := uc.categories.DistinctServiceCategories(ctx)
	if err != nil {
		log.Printf("[AdminCMSUsecase.EnsureSeedCategories] distinct error: %v", err)
		return inserted, err
	}
	var extra []string
	for _, raw := range distinct {
		slug := SlugifyCategory(raw)
		if slug == "" {
			continue
		}
		extra = append(extra, slug)
	}
	n, err := uc.categories.BackfillDistinct(ctx, extra)
	if err != nil {
		log.Printf("[AdminCMSUsecase.EnsureSeedCategories] backfill error: %v", err)
		return inserted, err
	}
	return inserted + n, nil
}

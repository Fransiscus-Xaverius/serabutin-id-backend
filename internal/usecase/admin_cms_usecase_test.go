package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

func assertDomainCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, code, domainErr.Code)
}

// Ticket 10: admin CMS (banners + categories) — red test. The usecase,
// validation, slugify, schedule gate, and taxonomy enforcement below do not
// exist yet; this test must fail until they are implemented.

type stubBannerRepo struct {
	rows   map[uint]*entity.Banner
	nextID uint
	list   []*entity.Banner
	public []*entity.Banner
}

func newStubBannerRepo() *stubBannerRepo {
	return &stubBannerRepo{rows: map[uint]*entity.Banner{}, nextID: 1}
}

func (s *stubBannerRepo) Create(_ context.Context, b *entity.Banner) error {
	b.ID = s.nextID
	s.nextID++
	b.CreatedAt = time.Now()
	s.rows[b.ID] = b
	return nil
}

func (s *stubBannerRepo) FindByID(_ context.Context, id uint) (*entity.Banner, error) {
	b, ok := s.rows[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "BANNER_NOT_FOUND", "banner not found")
	}
	return b, nil
}

func (s *stubBannerRepo) ListForAdmin(_ context.Context) ([]*entity.Banner, error) {
	return s.list, nil
}

func (s *stubBannerRepo) ListPublic(_ context.Context, _ string, _ time.Time) ([]*entity.Banner, error) {
	return s.public, nil
}

func (s *stubBannerRepo) Update(_ context.Context, b *entity.Banner) error {
	if _, ok := s.rows[b.ID]; !ok {
		return apperror.NewDomainError(apperror.ErrNotFound, "BANNER_NOT_FOUND", "banner not found")
	}
	s.rows[b.ID] = b
	return nil
}

func (s *stubBannerRepo) Delete(_ context.Context, id uint) error {
	if _, ok := s.rows[id]; !ok {
		return apperror.NewDomainError(apperror.ErrNotFound, "BANNER_NOT_FOUND", "banner not found")
	}
	delete(s.rows, id)
	return nil
}

type stubCategoryRepo struct {
	rows     map[uint]*entity.Category
	bySlug   map[string]*entity.Category
	nextID   uint
	distinct []string
}

func newStubCategoryRepo() *stubCategoryRepo {
	return &stubCategoryRepo{rows: map[uint]*entity.Category{}, bySlug: map[string]*entity.Category{}, nextID: 1}
}

func (s *stubCategoryRepo) Create(_ context.Context, c *entity.Category) error {
	if _, dup := s.bySlug[c.Slug]; dup {
		return apperror.NewDomainError(apperror.ErrAlreadyExists, "CATEGORY_EXISTS", "category slug already exists")
	}
	c.ID = s.nextID
	s.nextID++
	c.CreatedAt = time.Now()
	s.rows[c.ID] = c
	s.bySlug[c.Slug] = c
	return nil
}

func (s *stubCategoryRepo) FindByID(_ context.Context, id uint) (*entity.Category, error) {
	c, ok := s.rows[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
	}
	return c, nil
}

func (s *stubCategoryRepo) FindBySlug(_ context.Context, slug string) (*entity.Category, error) {
	c, ok := s.bySlug[slug]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
	}
	return c, nil
}

func (s *stubCategoryRepo) ListForAdmin(_ context.Context) ([]*entity.Category, error) {
	var out []*entity.Category
	for _, c := range s.rows {
		out = append(out, c)
	}
	return out, nil
}

func (s *stubCategoryRepo) ListPublic(_ context.Context) ([]*entity.Category, error) {
	var out []*entity.Category
	for _, c := range s.rows {
		if c.Active {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *stubCategoryRepo) ExistsActive(_ context.Context, slug string) (bool, error) {
	c, ok := s.bySlug[slug]
	return ok && c.Active, nil
}

func (s *stubCategoryRepo) Update(_ context.Context, c *entity.Category) error {
	old, ok := s.rows[c.ID]
	if !ok {
		return apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
	}
	if other, dup := s.bySlug[c.Slug]; dup && other.ID != c.ID {
		return apperror.NewDomainError(apperror.ErrAlreadyExists, "CATEGORY_EXISTS", "category slug already exists")
	}
	delete(s.bySlug, old.Slug)
	s.rows[c.ID] = c
	s.bySlug[c.Slug] = c
	return nil
}

func (s *stubCategoryRepo) Delete(_ context.Context, id uint) error {
	c, ok := s.rows[id]
	if !ok {
		return apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
	}
	delete(s.rows, id)
	delete(s.bySlug, c.Slug)
	return nil
}

func (s *stubCategoryRepo) DistinctServiceCategories(_ context.Context) ([]string, error) {
	return s.distinct, nil
}

func (s *stubCategoryRepo) BackfillDistinct(_ context.Context, slugs []string) (int64, error) {
	var n int64
	for _, slug := range slugs {
		if _, ok := s.bySlug[slug]; ok {
			continue
		}
		c := &entity.Category{ID: s.nextID, Slug: slug, Name: slug, Active: true}
		s.nextID++
		s.rows[c.ID] = c
		s.bySlug[slug] = c
		n++
	}
	return n, nil
}

func TestAdminCMS_CreateBannerRequiresTitleAndImage(t *testing.T) {
	uc := NewAdminCMSUsecase(newStubBannerRepo(), newStubCategoryRepo())
	_, err := uc.CreateBanner(context.Background(), "", "", "", "home", 0, true, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestAdminCMS_BannerScheduleWindowRejected(t *testing.T) {
	uc := NewAdminCMSUsecase(newStubBannerRepo(), newStubCategoryRepo())
	now := time.Now()
	start := now.Add(2 * time.Hour)
	end := now.Add(1 * time.Hour)
	_, err := uc.CreateBanner(context.Background(), "Promo", "https://img/x.jpg", "", "home", 0, true, &start, &end)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ends_at must be after starts_at")
}

func TestAdminCMS_CreateCategoryAutoSlugAndDup(t *testing.T) {
	uc := NewAdminCMSUsecase(newStubBannerRepo(), newStubCategoryRepo())
	c, err := uc.CreateCategory(context.Background(), "", "Bersih Rumah", "", 0, true)
	require.NoError(t, err)
	assert.Equal(t, "bersih-rumah", c.Slug)

	_, err = uc.CreateCategory(context.Background(), "bersih-rumah", "Duplikat", "", 0, true)
	require.Error(t, err)
	assertDomainCode(t, err, "CATEGORY_EXISTS")
}

func TestAdminCMS_DeactivateHidesPubliclyButKeepsRow(t *testing.T) {
	banners := newStubBannerRepo()
	cats := newStubCategoryRepo()
	uc := NewAdminCMSUsecase(banners, cats)
	c, err := uc.CreateCategory(context.Background(), "cleaning", "Cleaning", "", 0, true)
	require.NoError(t, err)

	active := false
	updated, err := uc.UpdateCategory(context.Background(), c.ID, CategoryPatch{Active: &active})
	require.NoError(t, err)
	assert.False(t, updated.Active)

	rows, err := uc.ListPublicCategories(context.Background())
	require.NoError(t, err)
	assert.Empty(t, rows)

	adminRows, err := uc.ListCategoriesForAdmin(context.Background())
	require.NoError(t, err)
	assert.Len(t, adminRows, 1)
}

func TestAdminCMS_UnknownServiceCategoryRejected(t *testing.T) {
	cats := newStubCategoryRepo()
	require.NoError(t, cats.Create(context.Background(), &entity.Category{Slug: "cleaning", Name: "Cleaning", Active: true}))
	uc := NewAdminCMSUsecase(newStubBannerRepo(), cats)

	require.NoError(t, uc.ValidateServiceCategory(context.Background(), "cleaning"))
	err := uc.ValidateServiceCategory(context.Background(), "nope")
	require.Error(t, err)
	assertDomainCode(t, err, "UNKNOWN_CATEGORY")
}

func TestAdminCMS_Slugify(t *testing.T) {
	assert.Equal(t, "bersih-rumah", SlugifyCategory("Bersih Rumah"))
	assert.Equal(t, "ac-service", SlugifyCategory("AC_Service!!"))
	assert.Equal(t, "a-b-c", SlugifyCategory("  A  B   C  "))
}

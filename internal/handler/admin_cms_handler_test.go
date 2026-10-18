package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// Ticket 10: admin CMS endpoints — banner CRUD + category CRUD with
// mandatory `reason` on mutations, plus the public storefront reads.

type stubCMSBannerRepo struct {
	rows   map[uint]*entity.Banner
	nextID uint
}

func newStubCMSBannerRepo() *stubCMSBannerRepo {
	return &stubCMSBannerRepo{rows: map[uint]*entity.Banner{}, nextID: 1}
}

func (s *stubCMSBannerRepo) Create(_ context.Context, b *entity.Banner) error {
	b.ID = s.nextID
	s.nextID++
	b.CreatedAt = time.Now()
	s.rows[b.ID] = b
	return nil
}

func (s *stubCMSBannerRepo) FindByID(_ context.Context, id uint) (*entity.Banner, error) {
	b, ok := s.rows[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "BANNER_NOT_FOUND", "banner not found")
	}
	return b, nil
}

func (s *stubCMSBannerRepo) ListForAdmin(_ context.Context) ([]*entity.Banner, error) {
	var out []*entity.Banner
	for _, b := range s.rows {
		out = append(out, b)
	}
	return out, nil
}

func (s *stubCMSBannerRepo) ListPublic(_ context.Context, _ string, _ time.Time) ([]*entity.Banner, error) {
	var out []*entity.Banner
	for _, b := range s.rows {
		if b.Active {
			out = append(out, b)
		}
	}
	return out, nil
}

func (s *stubCMSBannerRepo) Update(_ context.Context, b *entity.Banner) error {
	if _, ok := s.rows[b.ID]; !ok {
		return apperror.NewDomainError(apperror.ErrNotFound, "BANNER_NOT_FOUND", "banner not found")
	}
	s.rows[b.ID] = b
	return nil
}

func (s *stubCMSBannerRepo) Delete(_ context.Context, id uint) error {
	if _, ok := s.rows[id]; !ok {
		return apperror.NewDomainError(apperror.ErrNotFound, "BANNER_NOT_FOUND", "banner not found")
	}
	delete(s.rows, id)
	return nil
}

type stubCMSCategoryRepo struct {
	rows   map[uint]*entity.Category
	bySlug map[string]*entity.Category
	nextID uint
}

func newStubCMSCategoryRepo() *stubCMSCategoryRepo {
	return &stubCMSCategoryRepo{rows: map[uint]*entity.Category{}, bySlug: map[string]*entity.Category{}, nextID: 1}
}

func (s *stubCMSCategoryRepo) Create(_ context.Context, c *entity.Category) error {
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

func (s *stubCMSCategoryRepo) FindByID(_ context.Context, id uint) (*entity.Category, error) {
	c, ok := s.rows[id]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
	}
	return c, nil
}

func (s *stubCMSCategoryRepo) FindBySlug(_ context.Context, slug string) (*entity.Category, error) {
	c, ok := s.bySlug[slug]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
	}
	return c, nil
}

func (s *stubCMSCategoryRepo) ListForAdmin(_ context.Context) ([]*entity.Category, error) {
	var out []*entity.Category
	for _, c := range s.rows {
		out = append(out, c)
	}
	return out, nil
}

func (s *stubCMSCategoryRepo) ListPublic(_ context.Context) ([]*entity.Category, error) {
	var out []*entity.Category
	for _, c := range s.rows {
		if c.Active {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *stubCMSCategoryRepo) ExistsActive(_ context.Context, slug string) (bool, error) {
	c, ok := s.bySlug[slug]
	return ok && c.Active, nil
}

func (s *stubCMSCategoryRepo) Update(_ context.Context, c *entity.Category) error {
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

func (s *stubCMSCategoryRepo) Delete(_ context.Context, id uint) error {
	c, ok := s.rows[id]
	if !ok {
		return apperror.NewDomainError(apperror.ErrNotFound, "CATEGORY_NOT_FOUND", "category not found")
	}
	delete(s.rows, id)
	delete(s.bySlug, c.Slug)
	return nil
}

func (s *stubCMSCategoryRepo) DistinctServiceCategories(_ context.Context) ([]string, error) {
	return []string{"cleaning"}, nil
}

func (s *stubCMSCategoryRepo) BackfillDistinct(_ context.Context, _ []string) (int64, error) {
	return 0, nil
}

func newAdminCMSTestApp(banners *stubCMSBannerRepo, cats *stubCMSCategoryRepo) *fiber.App {
	uc := usecase.NewAdminCMSUsecase(banners, cats)
	h := NewAdminHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetAdminCMSUC(uc)
	public := NewCmsHandler(uc)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 7)
		return c.Next()
	})
	app.Get("/api/v1/admin/banners", h.ListBanners)
	app.Post("/api/v1/admin/banners", h.CreateBanner)
	app.Patch("/api/v1/admin/banners/:id", h.UpdateBanner)
	app.Delete("/api/v1/admin/banners/:id", h.DeleteBanner)
	app.Get("/api/v1/admin/categories", h.ListCategories)
	app.Post("/api/v1/admin/categories", h.CreateCategory)
	app.Patch("/api/v1/admin/categories/:id", h.UpdateCategory)
	app.Delete("/api/v1/admin/categories/:id", h.DeleteCategory)
	app.Get("/api/v1/banners", public.ListBanners)
	app.Get("/api/v1/categories", public.ListCategories)
	return app
}

func doCMSRequest(t *testing.T, app *fiber.App, method, url, body string) (int, map[string]interface{}) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return resp.StatusCode, decoded
}

func TestAdminCMS_CreateBannerRequiresReason(t *testing.T) {
	t.Parallel()
	app := newAdminCMSTestApp(newStubCMSBannerRepo(), newStubCMSCategoryRepo())
	status, body := doCMSRequest(t, app, "POST", "/api/v1/admin/banners",
		`{"title":"Promo","image_url":"https://img/x.jpg"}`)
	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.Equal(t, "VALIDATION_ERROR", body["code"])
}

func TestAdminCMS_CreateBannerReturns201(t *testing.T) {
	t.Parallel()
	app := newAdminCMSTestApp(newStubCMSBannerRepo(), newStubCMSCategoryRepo())
	status, body := doCMSRequest(t, app, "POST", "/api/v1/admin/banners",
		`{"title":"Promo","image_url":"https://img/x.jpg","placement":"home","reason":"campaign oktober"}`)
	assert.Equal(t, fiber.StatusCreated, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "Promo", data["title"])
	assert.Equal(t, "home", data["placement"])
	assert.Equal(t, true, data["active"])
}

// F1: `active` is a *bool in the create DTO, so an explicit false must reach
// the usecase unchanged while an omitted field still defaults to published.
func TestAdminCMS_CreateBannerExplicitInactiveStaysInactive(t *testing.T) {
	t.Parallel()
	app := newAdminCMSTestApp(newStubCMSBannerRepo(), newStubCMSCategoryRepo())
	status, body := doCMSRequest(t, app, "POST", "/api/v1/admin/banners",
		`{"title":"Draf","image_url":"https://img/x.jpg","active":false,"reason":"belum siap tayang"}`)
	assert.Equal(t, fiber.StatusCreated, status)
	assert.Equal(t, false, body["data"].(map[string]interface{})["active"])
}

func TestAdminCMS_CreateCategoryInactiveAndDefault(t *testing.T) {
	t.Parallel()
	app := newAdminCMSTestApp(newStubCMSBannerRepo(), newStubCMSCategoryRepo())

	status, body := doCMSRequest(t, app, "POST", "/api/v1/admin/categories",
		`{"slug":"draf-kategori","name":"Draf Kategori","active":false,"reason":"menunggu review"}`)
	assert.Equal(t, fiber.StatusCreated, status)
	assert.Equal(t, false, body["data"].(map[string]interface{})["active"])

	status, body = doCMSRequest(t, app, "POST", "/api/v1/admin/categories",
		`{"slug":"kategori-umum","name":"Kategori Umum","reason":"taksonomi baru"}`)
	assert.Equal(t, fiber.StatusCreated, status)
	assert.Equal(t, true, body["data"].(map[string]interface{})["active"])
}

func TestAdminCMS_UpdateBannerToggleRequiresReason(t *testing.T) {
	t.Parallel()
	banners := newStubCMSBannerRepo()
	app := newAdminCMSTestApp(banners, newStubCMSCategoryRepo())
	_, created := doCMSRequest(t, app, "POST", "/api/v1/admin/banners",
		`{"title":"Promo","image_url":"https://img/x.jpg","reason":"seed"}`)
	id := created["data"].(map[string]interface{})["id"]

	status, _ := doCMSRequest(t, app, "PATCH", "/api/v1/admin/banners/1", `{"active":false}`)
	_ = id
	assert.Equal(t, fiber.StatusBadRequest, status)

	status, body := doCMSRequest(t, app, "PATCH", "/api/v1/admin/banners/1",
		`{"active":false,"reason":"jeda tayang"}`)
	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, false, body["data"].(map[string]interface{})["active"])
}

func TestAdminCMS_DeleteBannerReasonViaQuery(t *testing.T) {
	t.Parallel()
	app := newAdminCMSTestApp(newStubCMSBannerRepo(), newStubCMSCategoryRepo())
	_, _ = doCMSRequest(t, app, "POST", "/api/v1/admin/banners",
		`{"title":"Promo","image_url":"https://img/x.jpg","reason":"seed"}`)

	status, _ := doCMSRequest(t, app, "DELETE", "/api/v1/admin/banners/1", "")
	assert.Equal(t, fiber.StatusBadRequest, status)

	status, _ = doCMSRequest(t, app, "DELETE", "/api/v1/admin/banners/1?reason=kadaluarsa", "")
	assert.Equal(t, fiber.StatusOK, status)

	status, _ = doCMSRequest(t, app, "DELETE", "/api/v1/admin/banners/1?reason=kadaluarsa", "")
	assert.Equal(t, fiber.StatusNotFound, status)
}

func TestAdminCMS_CreateCategoryDupSlug409(t *testing.T) {
	t.Parallel()
	app := newAdminCMSTestApp(newStubCMSBannerRepo(), newStubCMSCategoryRepo())
	status, _ := doCMSRequest(t, app, "POST", "/api/v1/admin/categories",
		`{"slug":"bersih-rumah","name":"Bersih Rumah","reason":"taksonomi baru"}`)
	assert.Equal(t, fiber.StatusCreated, status)

	status, body := doCMSRequest(t, app, "POST", "/api/v1/admin/categories",
		`{"slug":"bersih-rumah","name":"Duplikat","reason":"coba lagi"}`)
	assert.Equal(t, fiber.StatusConflict, status)
	assert.Equal(t, "CATEGORY_EXISTS", body["code"])
}

func TestAdminCMS_UpdateCategoryRequiresReason(t *testing.T) {
	t.Parallel()
	app := newAdminCMSTestApp(newStubCMSBannerRepo(), newStubCMSCategoryRepo())
	_, _ = doCMSRequest(t, app, "POST", "/api/v1/admin/categories",
		`{"slug":"cleaning","name":"Cleaning","reason":"seed"}`)

	status, _ := doCMSRequest(t, app, "PATCH", "/api/v1/admin/categories/1", `{"name":"Baru"}`)
	assert.Equal(t, fiber.StatusBadRequest, status)

	status, body := doCMSRequest(t, app, "PATCH", "/api/v1/admin/categories/1",
		`{"name":"Cleaning Rumah","sort_order":3,"reason":"rebrand"}`)
	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "Cleaning Rumah", data["name"])
	assert.Equal(t, float64(3), data["sort_order"])
}

func TestAdminCMS_DeleteCategoryKeepsServicesUnaffected(t *testing.T) {
	t.Parallel()
	app := newAdminCMSTestApp(newStubCMSBannerRepo(), newStubCMSCategoryRepo())
	_, _ = doCMSRequest(t, app, "POST", "/api/v1/admin/categories",
		`{"slug":"cleaning","name":"Cleaning","reason":"seed"}`)

	status, _ := doCMSRequest(t, app, "DELETE", "/api/v1/admin/categories/1",
		`{"reason":"digabung ke jasa lain"}`)
	assert.Equal(t, fiber.StatusOK, status)

	status, body := doCMSRequest(t, app, "GET", "/api/v1/categories", "")
	assert.Equal(t, fiber.StatusOK, status)
	assert.Empty(t, body["data"].(map[string]interface{})["data"].([]interface{}))
}

func TestAdminCMS_PublicReads(t *testing.T) {
	t.Parallel()
	app := newAdminCMSTestApp(newStubCMSBannerRepo(), newStubCMSCategoryRepo())
	_, _ = doCMSRequest(t, app, "POST", "/api/v1/admin/banners",
		`{"title":"Promo","image_url":"https://img/x.jpg","reason":"seed"}`)
	_, _ = doCMSRequest(t, app, "POST", "/api/v1/admin/categories",
		`{"slug":"cleaning","name":"Cleaning","reason":"seed"}`)

	status, body := doCMSRequest(t, app, "GET", "/api/v1/banners?placement=home", "")
	assert.Equal(t, fiber.StatusOK, status)
	assert.Len(t, body["data"].(map[string]interface{})["data"].([]interface{}), 1)

	status, body = doCMSRequest(t, app, "GET", "/api/v1/categories", "")
	assert.Equal(t, fiber.StatusOK, status)
	assert.Len(t, body["data"].(map[string]interface{})["data"].([]interface{}), 1)
}

package gormrepo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func setupCMSTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()
	container, err := testpostgres.Run(ctx, "postgres:16-alpine",
		testpostgres.WithDatabase("testdb"),
		testpostgres.WithUsername("test"),
		testpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.BannerModel{}, &model.CategoryModel{}))
	return db
}

// F1: banners are created as drafts from the CMS panel (`active:false`). The
// `default:true` column default must not override the explicit value, or a
// draft publishes itself and bypasses the review-before-publish control.
func TestBannerRepository_Create_PersistsExplicitInactive(t *testing.T) {
	db := setupCMSTestDB(t)
	repo := NewBannerRepository(db)
	ctx := context.Background()

	banner := &entity.Banner{
		Title:     "Draft promo akhir bulan",
		ImageURL:  "http://media.example.test/uploads/banner-draft.jpg",
		Placement: "home",
		Active:    false,
	}
	require.NoError(t, repo.Create(ctx, banner))

	stored, err := repo.FindByID(ctx, banner.ID)
	require.NoError(t, err)
	assert.False(t, stored.Active, "explicit active=false must persist")
	assert.False(t, stored.CreatedAt.IsZero(), "created_at must still be maintained")

	rows, err := repo.ListForAdmin(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.False(t, rows[0].Active)

	public, err := repo.ListPublic(ctx, "home", time.Now())
	require.NoError(t, err)
	assert.Empty(t, public, "an inactive banner must not be published")
}

func TestBannerRepository_Create_PersistsExplicitActive(t *testing.T) {
	db := setupCMSTestDB(t)
	repo := NewBannerRepository(db)
	ctx := context.Background()

	banner := &entity.Banner{
		Title:     "Promo aktif",
		ImageURL:  "http://media.example.test/uploads/banner-live.jpg",
		Placement: "home",
		Active:    true,
	}
	require.NoError(t, repo.Create(ctx, banner))

	stored, err := repo.FindByID(ctx, banner.ID)
	require.NoError(t, err)
	assert.True(t, stored.Active)

	public, err := repo.ListPublic(ctx, "home", time.Now())
	require.NoError(t, err)
	require.Len(t, public, 1)
	assert.Equal(t, banner.ID, public[0].ID)
}

// Same zero-value-with-default trap as F1: categories share the
// `default:true` column default on an `active` bool.
func TestCategoryRepository_Create_PersistsExplicitInactive(t *testing.T) {
	db := setupCMSTestDB(t)
	repo := NewCategoryRepository(db)
	ctx := context.Background()

	category := &entity.Category{Slug: "bersih-rumah", Name: "Bersih Rumah", SortOrder: 9, Active: false}
	require.NoError(t, repo.Create(ctx, category))

	stored, err := repo.FindByID(ctx, category.ID)
	require.NoError(t, err)
	assert.False(t, stored.Active, "explicit active=false must persist")
	assert.False(t, stored.CreatedAt.IsZero(), "created_at must still be maintained")

	ok, err := repo.ExistsActive(ctx, "bersih-rumah")
	require.NoError(t, err)
	assert.False(t, ok, "an inactive category must not be selectable")

	public, err := repo.ListPublic(ctx)
	require.NoError(t, err)
	assert.Empty(t, public)
}

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
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := testpostgres.Run(ctx,
		"postgres:16-alpine",
		testpostgres.WithDatabase("testdb"),
		testpostgres.WithUsername("test"),
		testpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := gorm.Open(gormpostgres.Open(connStr), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserModel{}))

	return db
}

func TestUserRepository_Create_Success(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Email:    "test@example.com",
		Password: "hashedpassword",
		Name:     "Test User",
	}

	err := repo.Create(context.Background(), user)

	require.NoError(t, err)
	assert.NotZero(t, user.ID)
	assert.False(t, user.CreatedAt.IsZero())
	assert.False(t, user.UpdatedAt.IsZero())
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, "Test User", user.Name)
}

func TestUserRepository_Create_DuplicateEmail_ReturnsErrDuplicateEmail(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user1 := &entity.User{
		Email:    "dup@example.com",
		Password: "password1",
		Name:     "User One",
	}
	err := repo.Create(context.Background(), user1)
	require.NoError(t, err)

	user2 := &entity.User{
		Email:    "dup@example.com",
		Password: "password2",
		Name:     "User Two",
	}
	err = repo.Create(context.Background(), user2)

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, apperror.ErrDuplicateEmail, domainErr.Err)
	assert.Equal(t, "EMAIL_EXISTS", domainErr.Code)
}

func TestUserRepository_FindByEmail_Found(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Email:    "find@example.com",
		Password: "hashedpassword",
		Name:     "Find Me",
	}
	require.NoError(t, repo.Create(context.Background(), user))

	found, err := repo.FindByEmail(context.Background(), "find@example.com")

	require.NoError(t, err)
	assert.Equal(t, user.ID, found.ID)
	assert.Equal(t, "find@example.com", found.Email)
	assert.Equal(t, "Find Me", found.Name)
}

func TestUserRepository_FindByEmail_NotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	_, err := repo.FindByEmail(context.Background(), "nonexistent@example.com")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, apperror.ErrNotFound, domainErr.Err)
	assert.Equal(t, "USER_NOT_FOUND", domainErr.Code)
}

func TestUserRepository_FindByID_Found(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Email:    "findbyid@example.com",
		Password: "hashedpassword",
		Name:     "Find By ID",
	}
	require.NoError(t, repo.Create(context.Background(), user))

	found, err := repo.FindByID(context.Background(), user.ID)

	require.NoError(t, err)
	assert.Equal(t, user.ID, found.ID)
	assert.Equal(t, "findbyid@example.com", found.Email)
	assert.Equal(t, "Find By ID", found.Name)
}

func TestUserRepository_FindByID_NotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	_, err := repo.FindByID(context.Background(), 99999)

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, apperror.ErrNotFound, domainErr.Err)
	assert.Equal(t, "USER_NOT_FOUND", domainErr.Code)
}

func TestUserRepository_ExistsByEmail_True(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Email:    "exists@example.com",
		Password: "hashedpassword",
		Name:     "Exists User",
	}
	require.NoError(t, repo.Create(context.Background(), user))

	exists, err := repo.ExistsByEmail(context.Background(), "exists@example.com")

	require.NoError(t, err)
	assert.True(t, exists)
}

func TestUserRepository_ExistsByEmail_False(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	exists, err := repo.ExistsByEmail(context.Background(), "nope@example.com")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestUserRepository_FindByWhatsApp_Found(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Email:    "wa@example.com",
		Password: "hashedpassword",
		Name:     "WA User",
		WhatsApp: "081234567890",
		Role:     "customer",
	}
	require.NoError(t, repo.Create(context.Background(), user))

	found, err := repo.FindByWhatsApp(context.Background(), "081234567890")

	require.NoError(t, err)
	assert.Equal(t, user.ID, found.ID)
	assert.Equal(t, "081234567890", found.WhatsApp)
}

func TestUserRepository_FindByWhatsApp_NotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	_, err := repo.FindByWhatsApp(context.Background(), "089999999999")

	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, apperror.ErrNotFound, domainErr.Err)
}

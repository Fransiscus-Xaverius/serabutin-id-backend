# Auth Integration — Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the serabutin-id-backend with WhatsApp login, account lockout, audit logging, PII redaction, and updated registration fields.

**Architecture:** Clean Architecture — domain entities, repository interfaces, usecase orchestration, handler HTTP layer, middleware for audit logging. All new code follows existing patterns and AGENT.md conventions.

**Tech Stack:** Go 1.25, GoFiber v3, GORM, Redis (go-redis), testify (testing), testcontainers-go (integration tests)

**Spec:** `docs/superpowers/specs/2026-05-29-auth-integration-spec.md`

---

### Task 1: PII Redaction Utility

**Files:**
- Create: `pkg/redact/redact.go`
- Test: `pkg/redact/redact_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/redact/redact_test.go`:

```go
package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactEmail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{"standard", "john.doe@example.com", "j***@example.com"},
		{"short", "a@b.com", "a***@b.com"},
		{"empty", "", "***@"},
		{"no_at", "invalid", "***@"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RedactEmail(tt.email))
		})
	}
}

func TestRedactPhone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		phone    string
		expected string
	}{
		{"standard", "081234567890", "08******7890"},
		{"short", "0812345678", "08******5678"},
		{"empty", "", "***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RedactPhone(tt.phone))
		})
	}
}

func TestRedactName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single", "John", "J***"},
		{"full", "John Doe", "J*** D***"},
		{"three", "John Paul Doe", "J*** P*** D***"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, RedactName(tt.input))
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/redact/... -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write implementation**

Create `pkg/redact/redact.go`:

```go
package redact

import "strings"

func RedactEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return "***@"
	}
	if len(parts[0]) == 0 {
		return "***@" + parts[1]
	}
	first := string(parts[0][0])
	return first + "***@" + parts[1]
}

func RedactPhone(phone string) string {
	if len(phone) == 0 {
		return "***"
	}
	if len(phone) <= 8 {
		return "***"
	}
	return phone[:2] + "******" + phone[len(phone)-4:]
}

func RedactName(name string) string {
	if name == "" {
		return ""
	}
	words := strings.Fields(name)
	result := make([]string, len(words))
	for i, word := range words {
		if len(word) == 0 {
			result[i] = ""
			continue
		}
		result[i] = string(word[0]) + "***"
	}
	return strings.Join(result, " ")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/redact/... -v`
Expected: PASS (all 11 tests)

- [ ] **Step 5: Commit**

```bash
git add pkg/redact/
git commit -m "feat: add PII redaction utility (email, phone, name)"
```

---

### Task 2: Config — Add Login Limiter Fields

**Files:**
- Modify: `internal/infrastructure/config/config.go`

- [ ] **Step 1: Write the failing test**

Create `internal/infrastructure/config/config_test.go`:

```go
package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DefaultLoginLimiterValues(t *testing.T) {
	t.Parallel()
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.LoginMaxAttempts)
	assert.Equal(t, 15, cfg.LoginLockoutMinutes)
}

func TestLoad_CustomLoginLimiterValues(t *testing.T) {
	t.Parallel()
	os.Setenv("LOGIN_MAX_ATTEMPTS", "3")
	os.Setenv("LOGIN_LOCKOUT_MINUTES", "30")
	t.Cleanup(func() {
		os.Unsetenv("LOGIN_MAX_ATTEMPTS")
		os.Unsetenv("LOGIN_LOCKOUT_MINUTES")
	})

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 3, cfg.LoginMaxAttempts)
	assert.Equal(t, 30, cfg.LoginLockoutMinutes)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/infrastructure/config/... -v`
Expected: FAIL — fields don't exist on Config struct

- [ ] **Step 3: Write implementation**

Modify `internal/infrastructure/config/config.go`:

Add to `Config` struct:
```go
	LoginMaxAttempts    int
	LoginLockoutMinutes int
```

Add to `Load()` function (before `var cfg Config`):
```go
	viper.SetDefault("LOGIN_MAX_ATTEMPTS", 5)
	viper.SetDefault("LOGIN_LOCKOUT_MINUTES", 15)
```

Add after `cfg.BcryptCost = viper.GetInt("BCRYPT_COST")`:
```go
	cfg.LoginMaxAttempts = viper.GetInt("LOGIN_MAX_ATTEMPTS")
	cfg.LoginLockoutMinutes = viper.GetInt("LOGIN_LOCKOUT_MINUTES")
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/infrastructure/config/... -v`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
git add internal/infrastructure/config/
git commit -m "feat: add login limiter config fields (max attempts, lockout duration)"
```

---

### Task 3: User Entity + Model — Add WhatsApp, Role, ConsentAt

**Files:**
- Modify: `internal/domain/entity/user.go`
- Modify: `internal/repository/model/user_model.go`

- [ ] **Step 1: Write the failing test**

Create `internal/domain/entity/user_test.go`:

```go
package entity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUser_NewFields(t *testing.T) {
	t.Parallel()
	now := time.Now()
	user := &User{
		Email:     "test@example.com",
		Password:  "hashed",
		Name:      "Test User",
		WhatsApp:  "081234567890",
		Role:      "customer",
		ConsentAt: &now,
	}

	assert.Equal(t, "081234567890", user.WhatsApp)
	assert.Equal(t, "customer", user.Role)
	assert.NotNil(t, user.ConsentAt)
	assert.Equal(t, now, *user.ConsentAt)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/entity/... -v`
Expected: FAIL — WhatsApp, Role, ConsentAt fields don't exist

- [ ] **Step 3: Write implementation**

Modify `internal/domain/entity/user.go`:

```go
package entity

import "time"

type User struct {
	ID        uint
	Email     string
	Password  string
	Name      string
	WhatsApp  string
	Role      string
	ConsentAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}
```

Modify `internal/repository/model/user_model.go`:

```go
package model

import (
	"time"

	"gorm.io/gorm"
)

type UserModel struct {
	ID        uint           `gorm:"primaryKey"`
	Email     string         `gorm:"uniqueIndex;not null;size:255"`
	Password  string         `gorm:"not null"`
	Name      string         `gorm:"size:100;not null"`
	WhatsApp  string         `gorm:"uniqueIndex;not null;size:20"`
	Role      string         `gorm:"size:20;default:customer;not null"`
	ConsentAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (UserModel) TableName() string {
	return "users"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/entity/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/entity/user.go internal/domain/entity/user_test.go internal/repository/model/user_model.go
git commit -m "feat: add WhatsApp, Role, ConsentAt to User entity and model"
```

---

### Task 4: UserRepository — Add FindByWhatsApp

**Files:**
- Modify: `internal/domain/repository/user_repository.go`
- Modify: `internal/repository/gorm/user_repository.go`
- Test: `internal/repository/gorm/user_repository_test.go` (add new tests)

- [ ] **Step 1: Write the failing test**

Add to `internal/repository/gorm/user_repository_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/repository/gorm/... -v -run "TestUserRepository_FindByWhatsApp"`
Expected: FAIL — FindByWhatsApp method doesn't exist

- [ ] **Step 3: Write implementation**

Modify `internal/domain/repository/user_repository.go`:

Add to interface:
```go
	FindByWhatsApp(ctx context.Context, whatsapp string) (*entity.User, error)
```

Modify `internal/repository/gorm/user_repository.go`:

Update `Create` to map new fields:
```go
func (r *UserRepository) Create(ctx context.Context, user *entity.User) error {
	m := &model.UserModel{
		Email:     user.Email,
		Password:  user.Password,
		Name:      user.Name,
		WhatsApp:  user.WhatsApp,
		Role:      user.Role,
		ConsentAt: user.ConsentAt,
	}
	// ... rest unchanged
}
```

Update `toEntity` to map new fields:
```go
func (r *UserRepository) toEntity(m *model.UserModel) *entity.User {
	return &entity.User{
		ID:        m.ID,
		Email:     m.Email,
		Password:  m.Password,
		Name:      m.Name,
		WhatsApp:  m.WhatsApp,
		Role:      m.Role,
		ConsentAt: m.ConsentAt,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
```

Add new method:
```go
func (r *UserRepository) FindByWhatsApp(ctx context.Context, whatsapp string) (*entity.User, error) {
	var m model.UserModel
	err := r.db.WithContext(ctx).Where("whatsapp = ?", whatsapp).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperror.NewDomainError(apperror.ErrNotFound, "USER_NOT_FOUND", "user not found")
		}
		return nil, apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
	}
	return r.toEntity(&m), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/repository/gorm/... -v -run "TestUserRepository_FindByWhatsApp"`
Expected: PASS (requires Docker for testcontainers)

- [ ] **Step 5: Commit**

```bash
git add internal/domain/repository/user_repository.go internal/repository/gorm/user_repository.go internal/repository/gorm/user_repository_test.go
git commit -m "feat: add FindByWhatsApp to user repository"
```

---

### Task 5: Login Limiter Service

**Files:**
- Create: `internal/domain/service/login_limiter.go`
- Test: `internal/domain/service/login_limiter_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/domain/service/login_limiter_test.go`:

```go
package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rediscontainer "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()
	redisContainer, err := rediscontainer.Run(ctx,
		"redis:7-alpine",
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithOccurrence(1).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = redisContainer.Terminate(ctx) })

	addr, err := redisContainer.Endpoint(ctx, "")
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{Addr: addr})
	_, err = client.Ping(ctx).Result()
	require.NoError(t, err)
	return client
}

func TestLoginLimiter_IsLocked_InitialNotLocked(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	limiter := NewLoginLimiter(client, 5, 15*time.Minute)

	locked, err := limiter.IsLocked(context.Background(), "test@example.com")
	require.NoError(t, err)
	assert.False(t, locked)
}

func TestLoginLimiter_RecordFailure_LocksAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	limiter := NewLoginLimiter(client, 3, 15*time.Minute)

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		require.NoError(t, limiter.RecordFailure(ctx, "test@example.com"))
	}

	locked, err := limiter.IsLocked(ctx, "test@example.com")
	require.NoError(t, err)
	assert.True(t, locked)
}

func TestLoginLimiter_RemainingAttempts(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	limiter := NewLoginLimiter(client, 5, 15*time.Minute)

	ctx := context.Background()
	require.NoError(t, limiter.RecordFailure(ctx, "test@example.com"))
	require.NoError(t, limiter.RecordFailure(ctx, "test@example.com"))

	remaining, err := limiter.RemainingAttempts(ctx, "test@example.com")
	require.NoError(t, err)
	assert.Equal(t, 3, remaining)
}

func TestLoginLimiter_Reset_ClearsLockout(t *testing.T) {
	t.Parallel()
	client := setupTestRedis(t)
	limiter := NewLoginLimiter(client, 2, 15*time.Minute)

	ctx := context.Background()
	require.NoError(t, limiter.RecordFailure(ctx, "test@example.com"))
	require.NoError(t, limiter.RecordFailure(ctx, "test@example.com"))

	locked, err := limiter.IsLocked(ctx, "test@example.com")
	require.NoError(t, err)
	assert.True(t, locked)

	require.NoError(t, limiter.Reset(ctx, "test@example.com"))

	locked, err = limiter.IsLocked(ctx, "test@example.com")
	require.NoError(t, err)
	assert.False(t, locked)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/service/... -v -run "TestLoginLimiter"`
Expected: FAIL — LoginLimiter doesn't exist

- [ ] **Step 3: Write implementation**

Create `internal/domain/service/login_limiter.go`:

```go
package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

const loginAttemptsPrefix = "login_attempts:"

type LoginLimiter struct {
	client          *redis.Client
	maxAttempts     int
	lockoutDuration time.Duration
}

func NewLoginLimiter(client *redis.Client, maxAttempts int, lockoutDuration time.Duration) *LoginLimiter {
	return &LoginLimiter{
		client:          client,
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
	}
}

func (l *LoginLimiter) IsLocked(ctx context.Context, identifier string) (bool, error) {
	key := fmt.Sprintf("%s%s", loginAttemptsPrefix, identifier)
	count, err := l.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}
	return count >= int64(l.maxAttempts), nil
}

func (l *LoginLimiter) RecordFailure(ctx context.Context, identifier string) error {
	key := fmt.Sprintf("%s%s", loginAttemptsPrefix, identifier)
	pipe := l.client.Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, l.lockoutDuration)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}
	return nil
}

func (l *LoginLimiter) Reset(ctx context.Context, identifier string) error {
	key := fmt.Sprintf("%s%s", loginAttemptsPrefix, identifier)
	if err := l.client.Del(ctx, key).Err(); err != nil {
		return apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}
	return nil
}

func (l *LoginLimiter) RemainingAttempts(ctx context.Context, identifier string) (int, error) {
	key := fmt.Sprintf("%s%s", loginAttemptsPrefix, identifier)
	count, err := l.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return l.maxAttempts, nil
	}
	if err != nil {
		return 0, apperror.NewDomainError(apperror.ErrInternalServer, "REDIS_ERROR", err.Error())
	}
	remaining := l.maxAttempts - int(count)
	if remaining < 0 {
		return 0, nil
	}
	return remaining, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/service/... -v -run "TestLoginLimiter"`
Expected: PASS (requires Docker)

- [ ] **Step 5: Commit**

```bash
git add internal/domain/service/login_limiter.go internal/domain/service/login_limiter_test.go
git commit -m "feat: add login limiter service (Redis-backed account lockout)"
```

---

### Task 6: DTOs — Update RegisterRequest and LoginRequest

**Files:**
- Modify: `internal/dto/auth_dto.go`

- [ ] **Step 1: Write the failing test**

Create `internal/dto/auth_dto_test.go`:

```go
package dto

import (
	"testing"

	"github.com/serabutin/serabutin-id-backend/pkg/validator"
	"github.com/stretchr/testify/assert"
)

func TestRegisterRequest_Valid_ReturnsNoErrors(t *testing.T) {
	t.Parallel()
	req := RegisterRequest{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
		WhatsApp: "081234567890",
		Role:     "customer",
	}

	errors := validator.Validate(req)
	assert.Nil(t, errors)
}

func TestRegisterRequest_InvalidRole_ReturnsError(t *testing.T) {
	t.Parallel()
	req := RegisterRequest{
		Email:    "test@example.com",
		Password: "password123",
		Name:     "Test User",
		WhatsApp: "081234567890",
		Role:     "admin",
	}

	errors := validator.Validate(req)
	assert.NotNil(t, errors)
}

func TestLoginRequest_Valid_ReturnsNoErrors(t *testing.T) {
	t.Parallel()
	req := LoginRequest{
		Identifier: "test@example.com",
		Password:   "password123",
	}

	errors := validator.Validate(req)
	assert.Nil(t, errors)
}

func TestLoginRequest_MissingIdentifier_ReturnsError(t *testing.T) {
	t.Parallel()
	req := LoginRequest{
		Password: "password123",
	}

	errors := validator.Validate(req)
	assert.NotNil(t, errors)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/dto/... -v`
Expected: FAIL — RegisterRequest missing WhatsApp/Role fields, LoginRequest missing Identifier

- [ ] **Step 3: Write implementation**

Modify `internal/dto/auth_dto.go`:

```go
package dto

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Name     string `json:"name" validate:"required,min=2,max=100"`
	WhatsApp string `json:"whatsapp" validate:"required,numeric"`
	Role     string `json:"role" validate:"required,oneof=customer provider"`
}

type LoginRequest struct {
	Identifier string `json:"identifier" validate:"required"`
	Password   string `json:"password" validate:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token,omitempty"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	User         UserResponse `json:"user"`
}

type UserResponse struct {
	ID       uint   `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	WhatsApp string `json:"whatsapp"`
	Role     string `json:"role"`
}

type MessageResponse struct {
	Message string `json:"message"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/dto/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/dto/auth_dto.go internal/dto/auth_dto_test.go
git commit -m "feat: update DTOs for WhatsApp login and role-based registration"
```

---

### Task 7: AuthUsecase — Update Register and Login

**Files:**
- Modify: `internal/usecase/auth_usecase.go`
- Test: `internal/usecase/auth_usecase_test.go` (update existing tests, add new ones)

- [ ] **Step 1: Write the failing test**

Add to `internal/usecase/auth_usecase_test.go`:

```go
func TestAuthUsecase_Register_WithWhatsAppAndRole_Success(t *testing.T) {
	t.Parallel()
	userRepo := new(MockUserRepository)
	tokenRepo := new(MockTokenRepository)
	tokenService := setupTokenService(t)
	uc := NewAuthUsecase(userRepo, tokenRepo, tokenService, 4)

	userRepo.On("ExistsByEmail", mock.Anything, "test@example.com").Return(false, nil)
	userRepo.On("FindByWhatsApp", mock.Anything, "081234567890").Return((*entity.User)(nil), apperror.ErrNotFound)
	userRepo.On("Create", mock.Anything, mock.MatchedBy(func(u *entity.User) bool {
		return u.Email == "test@example.com" && u.WhatsApp == "081234567890" && u.Role == "customer" && u.ConsentAt != nil
	})).Return(nil).Run(func(args mock.Arguments) {
		u := args.Get(1).(*entity.User)
		u.ID = 1
		u.CreatedAt = time.Now()
		u.UpdatedAt = time.Now()
	})
	tokenRepo.On("StoreTokenPair", mock.Anything, mock.Anything, uint(1)).Return(nil)

	td, user, err := uc.Register(context.Background(), "test@example.com", "password123", "Test User", "081234567890", "customer")

	require.NoError(t, err)
	assert.NotNil(t, td)
	assert.Equal(t, "081234567890", user.WhatsApp)
	assert.Equal(t, "customer", user.Role)
	userRepo.AssertExpectations(t)
}

func TestAuthUsecase_Login_WithWhatsApp_Success(t *testing.T) {
	t.Parallel()
	userRepo := new(MockUserRepository)
	tokenRepo := new(MockTokenRepository)
	tokenService := setupTokenService(t)
	limiter := setupLoginLimiter(t)
	uc := NewAuthUsecase(userRepo, tokenRepo, tokenService, 4, limiter)

	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("password123"), 4)
	existingUser := &entity.User{
		ID: 1, Email: "", Password: string(hashedPw), Name: "WA User",
		WhatsApp: "081234567890", Role: "customer",
	}
	limiter.On("IsLocked", mock.Anything, "081234567890").Return(false, nil)
	userRepo.On("FindByWhatsApp", mock.Anything, "081234567890").Return(existingUser, nil)
	limiter.On("Reset", mock.Anything, "081234567890").Return(nil)
	tokenRepo.On("StoreTokenPair", mock.Anything, mock.Anything, uint(1)).Return(nil)

	td, user, err := uc.Login(context.Background(), "081234567890", "password123")

	require.NoError(t, err)
	assert.NotNil(t, td)
	assert.Equal(t, "WA User", user.Name)
	userRepo.AssertExpectations(t)
	limiter.AssertExpectations(t)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/... -v -run "TestAuthUsecase_Register_WithWhatsApp|TestAuthUsecase_Login_WithWhatsApp"`
Expected: FAIL — Register signature changed, LoginLimiter dependency missing

- [ ] **Step 3: Write implementation**

Modify `internal/usecase/auth_usecase.go`:

Update struct:
```go
type AuthUsecase struct {
	userRepo     repository.UserRepository
	tokenRepo    repository.TokenRepository
	tokenService *service.TokenService
	loginLimiter *service.LoginLimiter
	bcryptCost   int
}
```

Update constructor:
```go
func NewAuthUsecase(
	userRepo repository.UserRepository,
	tokenRepo repository.TokenRepository,
	tokenService *service.TokenService,
	loginLimiter *service.LoginLimiter,
	bcryptCost int,
) *AuthUsecase {
	return &AuthUsecase{
		userRepo:     userRepo,
		tokenRepo:    tokenRepo,
		tokenService: tokenService,
		loginLimiter: loginLimiter,
		bcryptCost:   bcryptCost,
	}
}
```

Update Register signature and body:
```go
func (uc *AuthUsecase) Register(ctx context.Context, email, password, name, whatsapp, role string) (*entity.TokenDetails, *entity.User, error) {
	exists, err := uc.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		log.Printf("[AuthUsecase.Register] check email exists error: %v", err)
		return nil, nil, err
	}
	if exists {
		return nil, nil, apperror.NewDomainError(apperror.ErrDuplicateEmail, "EMAIL_EXISTS", "email already registered")
	}

	waExists, err := uc.userRepo.FindByWhatsApp(ctx, whatsapp)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		log.Printf("[AuthUsecase.Register] check whatsapp exists error: %v", err)
		return nil, nil, err
	}
	if waExists != nil {
		return nil, nil, apperror.NewDomainError(apperror.ErrDuplicateEmail, "WHATSAPP_EXISTS", "whatsapp already registered")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), uc.bcryptCost)
	if err != nil {
		return nil, nil, apperror.NewDomainError(apperror.ErrInternalServer, "HASH_FAILED", "failed to hash password")
	}

	now := time.Now()
	user := &entity.User{
		Email:     email,
		Password:  string(hashedPassword),
		Name:      name,
		WhatsApp:  whatsapp,
		Role:      role,
		ConsentAt: &now,
	}

	if err := uc.userRepo.Create(ctx, user); err != nil {
		log.Printf("[AuthUsecase.Register] create user error: %v", err)
		return nil, nil, err
	}

	td, err := uc.tokenService.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		log.Printf("[AuthUsecase.Register] generate tokens error: %v", err)
		return nil, nil, err
	}

	if err := uc.tokenRepo.StoreTokenPair(ctx, td, user.ID); err != nil {
		log.Printf("[AuthUsecase.Register] store tokens error: %v", err)
		return nil, nil, err
	}

	return td, user, nil
}
```

Update Login:
```go
func (uc *AuthUsecase) Login(ctx context.Context, identifier, password string) (*entity.TokenDetails, *entity.User, error) {
	locked, err := uc.loginLimiter.IsLocked(ctx, identifier)
	if err != nil {
		log.Printf("[AuthUsecase.Login] check lockout error: %v", err)
	}
	if locked {
		return nil, nil, apperror.NewDomainError(apperror.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid email/whatsapp or password")
	}

	var user *entity.User
	if strings.Contains(identifier, "@") {
		user, err = uc.userRepo.FindByEmail(ctx, identifier)
	} else {
		user, err = uc.userRepo.FindByWhatsApp(ctx, identifier)
	}

	if err != nil {
		if errors.Is(err, apperror.ErrNotFound) {
			_ = uc.loginLimiter.RecordFailure(ctx, identifier)
			return nil, nil, apperror.NewDomainError(apperror.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid email/whatsapp or password")
		}
		log.Printf("[AuthUsecase.Login] find user error: %v", err)
		return nil, nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		_ = uc.loginLimiter.RecordFailure(ctx, identifier)
		return nil, nil, apperror.NewDomainError(apperror.ErrInvalidCredentials, "INVALID_CREDENTIALS", "invalid email/whatsapp or password")
	}

	_ = uc.loginLimiter.Reset(ctx, identifier)

	td, err := uc.tokenService.GenerateTokenPair(user.ID, user.Email)
	if err != nil {
		log.Printf("[AuthUsecase.Login] generate tokens error: %v", err)
		return nil, nil, err
	}

	if err := uc.tokenRepo.StoreTokenPair(ctx, td, user.ID); err != nil {
		log.Printf("[AuthUsecase.Login] store tokens error: %v", err)
		return nil, nil, err
	}

	return td, user, nil
}
```

Add `strings` import.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/usecase/... -v`
Expected: PASS (existing tests need mock updates too — the implementer should update all existing mock calls to match new signatures)

- [ ] **Step 5: Commit**

```bash
git add internal/usecase/auth_usecase.go internal/usecase/auth_usecase_test.go
git commit -m "feat: update auth usecase with WhatsApp login, role, and login limiter"
```

---

### Task 8: AuthHandler — Update Register and Login

**Files:**
- Modify: `internal/handler/auth_handler.go`
- Test: `internal/handler/auth_handler_test.go` (update existing tests)

- [ ] **Step 1: Write the failing test**

Add to `internal/handler/auth_handler_test.go`:

```go
func TestAuthHandler_Register_WithWhatsApp_Returns201(t *testing.T) {
	t.Parallel()
	app, mocks := setupTestApp(t)

	mocks.userRepo.On("ExistsByEmail", mock.Anything, "test@example.com").Return(false, nil)
	mocks.userRepo.On("FindByWhatsApp", mock.Anything, "081234567890").Return((*entity.User)(nil), apperror.ErrNotFound)
	mocks.userRepo.On("Create", mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		u := args.Get(1).(*entity.User)
		u.ID = 1
	})
	mocks.tokenRepo.On("StoreTokenPair", mock.Anything, mock.Anything, uint(1)).Return(nil)

	body := `{"email":"test@example.com","password":"password123","name":"Test","whatsapp":"081234567890","role":"customer"}`
	req := httptest.NewRequest("POST", "/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
}

func TestAuthHandler_Login_WithIdentifier_Returns200(t *testing.T) {
	t.Parallel()
	app, mocks := setupTestApp(t)

	hashedPw, _ := bcrypt.GenerateFromPassword([]byte("password123"), 4)
	user := &entity.User{ID: 1, Email: "test@example.com", Password: string(hashedPw), Name: "Test", WhatsApp: "081234567890", Role: "customer"}

	mocks.loginLimiter.On("IsLocked", mock.Anything, "test@example.com").Return(false, nil)
	mocks.userRepo.On("FindByEmail", mock.Anything, "test@example.com").Return(user, nil)
	mocks.loginLimiter.On("Reset", mock.Anything, "test@example.com").Return(nil)
	mocks.tokenRepo.On("StoreTokenPair", mock.Anything, mock.Anything, uint(1)).Return(nil)

	body := `{"identifier":"test@example.com","password":"password123"}`
	req := httptest.NewRequest("POST", "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/... -v -run "TestAuthHandler_Register_WithWhatsApp|TestAuthHandler_Login_WithIdentifier"`
Expected: FAIL — handler uses old DTO fields

- [ ] **Step 3: Write implementation**

Modify `internal/handler/auth_handler.go`:

Update Register:
```go
func (h *AuthHandler) Register(c fiber.Ctx) error {
	var req dto.RegisterRequest
	if err := c.Bind().Body(&req); err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	if !whatsappRegex.MatchString(req.WhatsApp) {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_WHATSAPP", "format nomor WhatsApp tidak valid")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	td, user, err := h.authUsecase.Register(c.Context(), req.Email, req.Password, req.Name, req.WhatsApp, req.Role)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := dto.AuthResponse{
		AccessToken:  td.AccessToken,
		RefreshToken: td.RefreshToken,
		ExpiresIn:    int64(time.Until(td.AtExpires).Seconds()),
		User: dto.UserResponse{
			ID:       user.ID,
			Email:    user.Email,
			Name:     user.Name,
			WhatsApp: user.WhatsApp,
			Role:     user.Role,
		},
	}

	return response.Success(c, fiber.StatusCreated, resp)
}
```

Update Login:
```go
func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req dto.LoginRequest
	if err := c.Bind().Body(&req); err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	td, user, err := h.authUsecase.Login(c.Context(), req.Identifier, req.Password)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	resp := dto.AuthResponse{
		AccessToken:  td.AccessToken,
		RefreshToken: td.RefreshToken,
		ExpiresIn:    int64(time.Until(td.AtExpires).Seconds()),
		User: dto.UserResponse{
			ID:       user.ID,
			Email:    user.Email,
			Name:     user.Name,
			WhatsApp: user.WhatsApp,
			Role:     user.Role,
		},
	}

	return response.Success(c, fiber.StatusOK, resp)
}
```

Add at package level:
```go
var whatsappRegex = regexp.MustCompile(`^08[1-9]\d{7,11}$`)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/handler/auth_handler.go internal/handler/auth_handler_test.go
git commit -m "feat: update auth handler with WhatsApp validation and new DTO fields"
```

---

### Task 9: Audit Logger Middleware

**Files:**
- Create: `internal/middleware/audit_logger.go`
- Test: `internal/middleware/audit_logger_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/middleware/audit_logger_test.go`:

```go
package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditLogger_LogsAuthEvents(t *testing.T) {
	t.Parallel()
	var logOutput strings.Builder
	logger := zerolog.New(&logOutput).With().Timestamp().Logger()

	app := fiber.New()
	app.Use(AuditLogger(logger))
	app.Post("/login", func(c fiber.Ctx) error {
		return c.SendStatus(200)
	})

	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"identifier":"test@example.com","password":"pass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "test-agent")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	log := logOutput.String()
	assert.Contains(t, log, "auth.login.success")
	assert.Contains(t, log, "test-agent")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/middleware/... -v -run "TestAuditLogger"`
Expected: FAIL — AuditLogger doesn't exist

- [ ] **Step 3: Write implementation**

Create `internal/middleware/audit_logger.go`:

```go
package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/serabutin/serabutin-id-backend/pkg/redact"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

func AuditLogger(log zerolog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start).Milliseconds()

		path := c.Path()
		if !strings.HasPrefix(path, "/api/v1/auth/") {
			return err
		}

		event := determineAuthEvent(path, c.Method(), err)
		if event == "" {
			return err
		}

		level := "info"
		if err != nil || strings.HasSuffix(event, ".failure") || strings.HasSuffix(event, ".triggered") {
			level = "warn"
		}

		evt := log.WithLevel(parseLevel(level)).
			Str("event", event).
			Str("request_id", requestctx.GetRequestID(c)).
			Str("ip", c.IP()).
			Str("user_agent", c.Get("User-Agent")).
			Int64("duration_ms", duration)

		userID := requestctx.GetUserID(c)
		if userID > 0 {
			evt.Uint("user_id", userID)
		}

		evt.Msg("auth event")

		return err
	}
}

func determineAuthEvent(path, method string, handlerErr error) string {
	switch {
	case strings.HasSuffix(path, "/auth/register") && method == "POST":
		if handlerErr != nil {
			return "auth.register.failure"
		}
		return "auth.register.success"
	case strings.HasSuffix(path, "/auth/login") && method == "POST":
		if handlerErr != nil {
			return "auth.login.failure"
		}
		return "auth.login.success"
	case strings.HasSuffix(path, "/auth/logout") && method == "POST":
		return "auth.logout"
	case strings.HasSuffix(path, "/auth/refresh") && method == "POST":
		if handlerErr != nil {
			return "auth.refresh.failure"
		}
		return "auth.refresh.success"
	}
	return ""
}

func parseLevel(level string) zerolog.Level {
	switch level {
	case "warn":
		return zerolog.WarnLevel
	default:
		return zerolog.InfoLevel
	}
}

func RedactIdentifier(identifier string) string {
	if strings.Contains(identifier, "@") {
		return redact.RedactEmail(identifier)
	}
	return redact.RedactPhone(identifier)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/middleware/... -v -run "TestAuditLogger"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/middleware/audit_logger.go internal/middleware/audit_logger_test.go
git commit -m "feat: add audit logger middleware for auth events"
```

---

### Task 10: Wire Dependencies in main.go

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Write implementation**

Modify `cmd/server/main.go`:

Add imports:
```go
	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
```

Add after `tokenService` creation:
```go
	loginLimiter := service.NewLoginLimiter(redisClient, cfg.LoginMaxAttempts, time.Duration(cfg.LoginLockoutMinutes)*time.Minute)
```

Update AuthUsecase creation:
```go
	authUsecase := usecase.NewAuthUsecase(userRepo, tokenRepo, tokenService, loginLimiter, cfg.BcryptCost)
```

Add audit logger middleware before auth routes:
```go
	api := app.Group("/api/v1")
	api.Use(middleware.AuditLogger(logger.Log))
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./cmd/server/...`
Expected: Success

- [ ] **Step 3: Run all non-Docker tests**

Run: `go test ./internal/domain/... ./internal/usecase/... ./internal/handler/... ./pkg/... ./internal/middleware/... -v -run "^Test(?!.*Limiter|.*Repository|.*RateLimiter)"`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire login limiter and audit logger in main"
```

---

## Summary

10 tasks producing:
- 4 new files (redact, login_limiter, audit_logger + tests)
- 6 modified files (entity, model, repository, usecase, handler, DTOs, config, main)
- ~40+ new/updated tests
- SOC2 compliance: account lockout, audit logging, PII redaction, generic error messages

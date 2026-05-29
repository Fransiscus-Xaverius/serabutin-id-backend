# Auth Integration Spec — Serabutin.id

> **Date:** 2026-05-29
> **Status:** Draft — pending user review
> **SOC2 Controls:** CC6.1, CC6.2, CC6.3, CC6.6, CC7.2, P2.1, P3.1, P4.1, C1.1

## Overview

Integrate authentication (login, register, logout, profile) into the serabutin.id platform. Backend changes add WhatsApp login, account lockout, audit logging, and PII redaction. Frontend converts an HTML mock into React components with a shared auth layout, password strength meter, and role selection.

## Architecture

```
Frontend (React)                          Backend (Go)
┌──────────────────┐                      ┌─────────────────────────┐
│  AuthLayout      │                      │  AuthHandler            │
│  ├─ LoginPage    │── POST /auth/login ──│  ├─ Login(identifier)   │
│  └─ RegisterPage │── POST /auth/register│  ├─ Register(user)      │
│                  │                      │  ├─ Logout()            │
│  AuthContext     │                      │  └─ RefreshToken()      │
│  ├─ login()      │                      │                         │
│  ├─ register()   │                      │  AuthUsecase            │
│  └─ logout()     │                      │  ├─ Login(identifier)   │
│                  │                      │  ├─ Register(user)      │
│  api.js          │                      │  ├─ Logout()            │
│  └─ authService  │                      │  └─ RefreshToken()      │
└──────────────────┘                      │                         │
                                          │  LoginLimiter (Redis)   │
                                          │  AuditLogger (zerolog)  │
                                          │  PII Redaction (pkg)    │
                                          │                         │
                                          │  UserRepository (GORM)  │
                                          │  TokenRepository (Redis)│
                                          └─────────────────────────┘
```

## Backend Changes

### 1. User Entity (`internal/domain/entity/user.go`)

**Current fields:** ID, Email, Password, Name, CreatedAt, UpdatedAt

**New fields:**
- `WhatsApp string` — Indonesian phone number (08xxxxxxxxxx)
- `Role string` — "customer" or "provider"
- `ConsentAt *time.Time` — Timestamp when user accepted terms (nil if not applicable)

**Constraints:**
- WhatsApp: unique index, non-null for registration
- Role: default "customer", enum validation
- ConsentAt: set only on registration, never modified after

### 2. User Model (`internal/repository/model/user_model.go`)

Add GORM columns:
- `WhatsApp string` — `uniqueIndex;not null;size:20`
- `Role string` — `size:20;default:customer;not null`
- `ConsentAt *time.Time` — nullable timestamp

AutoMigrate handles schema changes at startup.

### 3. UserRepository Interface (`internal/domain/repository/user_repository.go`)

**New method:**
```go
FindByWhatsApp(ctx context.Context, whatsapp string) (*entity.User, error)
```

**Updated method:**
```go
Create(ctx context.Context, user *entity.User) error
// Now expects WhatsApp, Role, ConsentAt to be populated on the entity
```

### 4. GORM UserRepository (`internal/repository/gorm/user_repository.go`)

- Implement `FindByWhatsApp` — queries by WhatsApp, returns `ErrNotFound` if not found
- Update `Create` — maps WhatsApp, Role, ConsentAt to UserModel
- Update `toEntity` — includes new fields in entity mapping

### 5. Login Limiter Service (`internal/domain/service/login_limiter.go`)

**Purpose:** Redis-backed account lockout to prevent brute force attacks.

**Interface:**
```go
type LoginLimiter struct {
    client     *redis.Client
    maxAttempts int
    lockoutDuration time.Duration
}

func (l *LoginLimiter) IsLocked(ctx context.Context, identifier string) (bool, error)
func (l *LoginLimiter) RecordFailure(ctx context.Context, identifier string) error
func (l *LoginLimiter) Reset(ctx context.Context, identifier string) error
func (l *LoginLimiter) RemainingAttempts(ctx context.Context, identifier string) (int, error)
```

**Redis key pattern:** `login_attempts:{identifier}` — stores attempt count as integer string
**TTL:** 15 minutes (resets after lockout window)
**Max attempts:** 5 (configurable via config)
**Lockout duration:** 15 minutes (configurable)

**Behavior:**
- `RecordFailure`: INCR counter, set TTL if first attempt
- `IsLocked`: check if count >= maxAttempts
- `Reset`: DEL key on successful login
- `RemainingAttempts`: maxAttempts - current count

### 6. AuthUsecase (`internal/usecase/auth_usecase.go`)

**Register(ctx, email, password, name, whatsapp, role) → (*TokenDetails, *User, error)**
- Check email exists → ErrDuplicateEmail
- Check WhatsApp exists → ErrDuplicateEmail ("whatsapp already registered")
- Validate role is "customer" or "provider"
- Bcrypt hash password (cost from config, default 12)
- Create user with WhatsApp, Role, ConsentAt = now
- Generate token pair, store in Redis
- Audit log: `auth.register.success` with redacted PII

**Login(ctx, identifier, password) → (*TokenDetails, *User, error)**
- Check if locked → return ErrInvalidCredentials (generic, never reveal lockout)
- Detect identifier type: if contains `@`, use FindByEmail; else use FindByWhatsApp
- If user not found → return ErrInvalidCredentials (generic)
- Compare bcrypt password
- If wrong → RecordFailure, return ErrInvalidCredentials
- If correct → Reset lockout counter, generate tokens, store in Redis
- Audit log: `auth.login.success` or `auth.login.failure` with redacted identifier

**Logout(ctx, accessToken, refreshToken) → error**
- Best-effort: blacklist access token, revoke refresh token
- Audit log: `auth.logout`

**RefreshToken(ctx, refreshTokenStr) → (*TokenDetails, error)**
- Validate refresh JWT, check Redis, revoke old, generate new pair
- Audit log: `auth.refresh.success` or `auth.refresh.failure`

### 7. DTOs (`internal/dto/auth_dto.go`)

**RegisterRequest:**
```go
type RegisterRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
    Name     string `json:"name" validate:"required,min=2,max=100"`
    WhatsApp string `json:"whatsapp" validate:"required,numeric,len=10..15"`
    Role     string `json:"role" validate:"required,oneof=customer provider"`
}
```

**LoginRequest:**
```go
type LoginRequest struct {
    Identifier string `json:"identifier" validate:"required"`
    Password   string `json:"password" validate:"required"`
}
```

**Note:** WhatsApp validation uses regex `^08[1-9]\d{7,11}$` (Indonesian format). The DTO `validate` tag checks `required` and `numeric`; the handler layer applies the regex pattern before calling usecase. Frontend uses the same regex for real-time validation.

### 8. AuthHandler (`internal/handler/auth_handler.go`)

**Register:**
- Bind RegisterRequest
- Validate DTO
- Validate WhatsApp format (Indonesian pattern)
- Call usecase.Register with all fields
- Return 201 with AuthResponse

**Login:**
- Bind LoginRequest
- Validate DTO
- Call usecase.Login with identifier + password
- Return 200 with AuthResponse

### 9. Audit Logger (`internal/middleware/audit_logger.go`)

**Purpose:** Structured audit logging for all auth events.

**Middleware function:** `AuditLogger(log zerolog.Logger) fiber.Handler`

**Events logged:**
- `auth.login.success` — level: info
- `auth.login.failure` — level: warn
- `auth.register.success` — level: info
- `auth.register.failure` — level: warn
- `auth.logout` — level: info
- `auth.lockout.triggered` — level: warn

**Fields per event:**
- `event` — event type string
- `request_id` — from request context
- `ip` — client IP
- `user_agent` — from request header
- `identifier` — redacted via PII redaction utility
- `user_id` — if available (for success events)
- `duration_ms` — request duration

### 10. PII Redaction (`pkg/redact/redact.go`)

```go
func RedactEmail(email string) string
// "john.doe@example.com" → "j***@example.com"

func RedactPhone(phone string) string
// "081234567890" → "08******7890"

func RedactName(name string) string
// "John Doe" → "J*** D***"
```

### 11. Config (`internal/infrastructure/config/config.go`)

**New fields:**
- `BcryptCost int` — default 12 (env: `BCRYPT_COST`). Tests override to 4 for speed.
- `LoginMaxAttempts int` — default 5 (env: `LOGIN_MAX_ATTEMPTS`)
- `LoginLockoutMinutes int` — default 15 (env: `LOGIN_LOCKOUT_MINUTES`)

### 12. Dependency Wiring (`cmd/server/main.go`)

New dependencies to wire:
- `LoginLimiter` — created with Redis client, max attempts, lockout duration
- `AuditLogger` — middleware using zerolog logger
- Pass `LoginLimiter` to `AuthUsecase` constructor
- Register `AuditLogger` middleware on auth routes

---

## Frontend Changes

### 1. AuthLayout (`src/layouts/AuthLayout.jsx`)

**Responsibility:** Shared visual shell for login and register pages.

**Structure:**
- Full viewport height, gradient background with decorative blobs (matches mock)
- Grid layout: left brand panel (hidden on mobile) + right auth card
- Left panel: headline, value proposition cards (Aman, Cepat, Jelas), floating mockup cards
- Right panel: white auth card with logo, title, subtitle, form content via `<Outlet />`
- Responsive: left panel hides on `< lg`, card takes full width on mobile
- Uses existing Tailwind config colors (primary, accent, soft-surface, warm-border)

### 2. LoginPage (`src/pages/LoginPage.jsx`)

**Responsibility:** Login form page.

**Fields:**
- Identifier (email or WhatsApp) — text input with user icon
- Password — password input with eye toggle
- Remember me — checkbox (stores email in localStorage)
- Forgot password — button shows toast "Link reset password akan dikirim ke email Anda"
- Submit button — "Masuk" with arrow icon
- Google login button — shows toast "Login Google belum terhubung" (placeholder)
- Link to register page

**Validation:**
- Identifier required
- Password required
- Error messages shown below each field with red border on input

**Behavior:**
- On submit: call `auth.login(identifier, password)`
- If remember me checked: store email in localStorage
- On success: redirect to `/` (home page)
- On failure: show error message from context

### 3. RegisterPage (`src/pages/RegisterPage.jsx`)

**Responsibility:** Registration form page.

**Fields:**
- Role toggle — two cards: "Pencari Jasa" (customer) / "Mitra Jasa" (provider)
- Name — text input with user icon
- WhatsApp — tel input with phone icon
- Email — email input with mail icon
- Password — password input with eye toggle + strength meter
- Terms checkbox — required, links to Terms of Service and Privacy Policy
- Submit button — "Buat Akun" with arrow icon
- Link to login page

**Password Strength Meter:**
- 4 criteria: length ≥ 8, mixed case, has digit, has special char
- Score 0-4 maps to: gray → red → amber → blue → green
- Width and color update on each keystroke
- Hint text below meter

**Validation:**
- Name: required, 2-100 chars
- WhatsApp: required, Indonesian format `^08[1-9]\d{7,11}$`
- Email: required, valid email format
- Password: required, min 8 chars
- Terms: required checkbox
- Error messages shown below each field

**Behavior:**
- On submit: call `auth.register(email, password, name)` with role and WhatsApp
- On success: show toast, redirect to `/login`
- On failure: show error message from context

### 4. AuthContext (`src/contexts/AuthContext.jsx`)

**State:**
- `user` — current user object (id, email, name, whatsapp, role)
- `isAuthenticated` — boolean
- `isLoading` — boolean (during async operations)
- `error` — string (last error message)

**Actions:**
- `login(emailOrWhatsapp, password)` — calls authService.login, stores tokens + user in localStorage
- `register(email, password, name, whatsapp, role)` — calls authService.register, stores tokens + user
- `logout()` — calls authService.logout, clears localStorage, clears user state

**Persistence:**
- `access_token` — localStorage
- `refresh_token` — localStorage
- `user` — localStorage (JSON stringified)
- On mount: restore from localStorage if tokens exist

### 5. AuthService (`src/services/auth.service.js`)

**Methods:**
- `register(email, password, name, whatsapp, role)` — POST `/api/v1/auth/register`
- `login(identifier, password)` — POST `/api/v1/auth/login`
- `logout(accessToken, refreshToken)` — POST `/api/v1/auth/logout`
- `refreshToken(refreshToken)` — POST `/api/v1/auth/refresh`
- `getMe(accessToken)` — GET `/api/v1/me`

### 6. Navbar Update (`src/components/Navbar.jsx`)

**Changes:**
- Replace hardcoded "Masuk" / "Daftar" buttons with conditional rendering based on auth state
- If not authenticated: show "Masuk" (links to `/login`) and "Daftar" (links to `/register`)
- If authenticated: show user name + profile icon (links to `/profile`) and "Keluar" button
- Mobile menu: same conditional rendering

### 7. Routes (`src/routes/index.jsx`)

**New routes:**
- `/login` — LoginPage (public, wrapped in AuthLayout)
- `/register` — RegisterPage (public, wrapped in AuthLayout)
- `/profile` — ProfilePage (protected by AuthGuard)

**Existing routes unchanged:**
- `/` — HomePage
- `/list-job` — ListJobPage
- `*` — NotFoundPage

### 8. main.jsx

**Change:** Wrap `<App />` with `<AuthProvider>` (outside ToastProvider)

---

## Error Messages (Security: Never Reveal Account Existence)

| Scenario | Message Shown |
|---|---|
| Wrong password | "Email/WhatsApp atau password salah" |
| Non-existent email | "Email/WhatsApp atau password salah" (same message) |
| Non-existent WhatsApp | "Email/WhatsApp atau password salah" (same message) |
| Account locked | "Email/WhatsApp atau password salah" (same message, no lockout reveal) |
| Duplicate email | "Email sudah terdaftar" |
| Duplicate WhatsApp | "Nomor WhatsApp sudah terdaftar" |
| Invalid WhatsApp format | "Format nomor WhatsApp tidak valid" |
| Password too short | "Password minimal 8 karakter" |
| Terms not accepted | "Anda perlu menyetujui syarat layanan" |

---

## API Contract (Backend → Frontend)

### POST /api/v1/auth/register

**Request:**
```json
{
  "email": "user@example.com",
  "password": "securepassword123",
  "name": "John Doe",
  "whatsapp": "081234567890",
  "role": "customer"
}
```

**Response (201):**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJ...",
    "refresh_token": "eyJ...",
    "expires_in": 900,
    "user": {
      "id": 1,
      "email": "user@example.com",
      "name": "John Doe",
      "whatsapp": "081234567890",
      "role": "customer"
    }
  },
  "request_id": "abc-123"
}
```

### POST /api/v1/auth/login

**Request:**
```json
{
  "identifier": "user@example.com",
  "password": "securepassword123"
}
```

**Response (200):** Same as register response.

### POST /api/v1/auth/logout

**Request:**
```json
{ "refresh_token": "eyJ..." }
```
**Header:** `Authorization: Bearer eyJ...`

**Response (200):**
```json
{
  "success": true,
  "message": "logged out successfully",
  "request_id": "abc-123"
}
```

### GET /api/v1/me

**Header:** `Authorization: Bearer eyJ...`

**Response (200):**
```json
{
  "success": true,
  "data": {
    "id": 1,
    "email": "user@example.com",
    "name": "John Doe",
    "whatsapp": "081234567890",
    "role": "customer"
  },
  "request_id": "abc-123"
}
```

---

## Test Strategy

### Backend
- Domain: login limiter service (lockout, reset, remaining attempts)
- Repository: FindByWhatsApp, Create with new fields
- Usecase: login with email, login with WhatsApp, login with wrong password, login when locked, register with all fields
- Handler: valid/invalid requests, WhatsApp format validation
- Middleware: audit logger logs correct events, PII is redacted
- Pkg: redact functions work correctly for all PII types

### Frontend
- AuthContext: login, register, logout, restore from localStorage
- LoginForm: validation, API call, error display, remember me
- RegisterForm: validation (all fields), password meter, role selection, API call
- AuthLayout: responsive behavior (left panel hidden on mobile)
- AuthButton: conditional rendering based on auth state

---

## Migration Strategy

1. Backend changes first (entity, model, repository, usecase, handler, middleware)
2. Run GORM AutoMigrate to add columns
3. Frontend changes second (layout, pages, forms, context, routes)
4. Both repos tested independently before integration

## Rollback

- Backend: revert entity/model changes, remove new columns via SQL migration if needed
- Frontend: revert route changes, restore old Navbar

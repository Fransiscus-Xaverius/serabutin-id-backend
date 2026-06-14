# Profile API + WhatsApp Validation + Audit Logger

**Date:** 2026-06-14
**Status:** Approved
**Scope:** Backend + Frontend

---

## 1. Overview

Extend the existing auth system with full profile management, proper Indonesian WhatsApp number validation/normalization, and wire the existing audit logger middleware.

---

## 2. Profile API

### 2.1 User Entity Extensions

Add fields to the existing `users` table:

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `bio` | string | max 500, optional | User biography/description |
| `avatar_url` | string | max 500, optional | Set via image upload endpoint |
| `address` | string | max 255, optional | Street address |
| `city` | string | max 100, optional | City name |
| `province` | string | max 100, optional | Province name |
| `skills` | string | max 500, optional | Comma-separated skill tags (provider only) |
| `service_areas` | string | max 500, optional | Comma-separated areas (provider only) |

### 2.2 Endpoints

#### GET /api/v1/me (replace existing stub)

- **Auth:** JWT required
- **Response:** Full profile including WhatsApp (own data)

```json
{
  "data": {
    "id": 1,
    "email": "user@example.com",
    "name": "Budi Santoso",
    "whatsapp": "+6287812345678",
    "role": "provider",
    "bio": "Experienced cleaner in Jakarta area",
    "avatar_url": "/uploads/abc123_original.jpg",
    "address": "Jl. Sudirman No. 1",
    "city": "Jakarta",
    "province": "DKI Jakarta",
    "skills": "cleaning,laundry,shopping",
    "service_areas": "Jakarta Selatan,Jakarta Pusat",
    "created_at": "2026-01-15T10:00:00Z"
  }
}
```

#### PATCH /api/v1/me

- **Auth:** JWT required
- **Body:** Partial update, only non-empty fields are updated
- **Excluded:** Password changes (separate future endpoint), WhatsApp changes (separate future endpoint), email changes (separate future endpoint)

```json
{
  "name": "Budi Santoso",
  "bio": "Updated bio",
  "city": "Bandung",
  "province": "Jawa Barat",
  "skills": "cleaning,errands",
  "service_areas": "Bandung Utara"
}
```

**Response:** Same as GET /api/v1/me

### 2.3 WhatsApp Visibility Rules

| Context | WhatsApp Visible? |
|---------|-------------------|
| GET /api/v1/me (own profile) | Yes |
| Auth responses (login/register) | Yes |
| Public provider profiles (future) | No |
| Service listings (future) | No |

### 2.4 DTO Changes

**UpdateProfileRequest:**
```go
type UpdateProfileRequest struct {
    Name         string `json:"name,omitempty" validate:"omitempty,min=2,max=100"`
    Bio          string `json:"bio,omitempty" validate:"omitempty,max=500"`
    Address      string `json:"address,omitempty" validate:"omitempty,max=255"`
    City         string `json:"city,omitempty" validate:"omitempty,max=100"`
    Province     string `json:"province,omitempty" validate:"omitempty,max=100"`
    Skills       string `json:"skills,omitempty" validate:"omitempty,max=500"`
    ServiceAreas string `json:"service_areas,omitempty" validate:"omitempty,max=500"`
}
```

**UserResponse** (for auth responses, own profile):
```go
type UserResponse struct {
    ID           uint   `json:"id"`
    Email        string `json:"email"`
    Name         string `json:"name"`
    WhatsApp     string `json:"whatsapp"`
    Role         string `json:"role"`
    Bio          string `json:"bio,omitempty"`
    AvatarURL    string `json:"avatar_url,omitempty"`
    Address      string `json:"address,omitempty"`
    City         string `json:"city,omitempty"`
    Province     string `json:"province,omitempty"`
    Skills       string `json:"skills,omitempty"`
    ServiceAreas string `json:"service_areas,omitempty"`
    CreatedAt    string `json:"created_at"`
}
```

**PublicProviderResponse** (for future public endpoints):
```go
type PublicProviderResponse struct {
    ID           uint   `json:"id"`
    Name         string `json:"name"`
    Role         string `json:"role"`
    Bio          string `json:"bio,omitempty"`
    AvatarURL    string `json:"avatar_url,omitempty"`
    City         string `json:"city,omitempty"`
    Province     string `json:"province,omitempty"`
    Skills       string `json:"skills,omitempty"`
    ServiceAreas string `json:"service_areas,omitempty"`
}
```

---

## 3. WhatsApp Validation & Normalization

### 3.1 Validation Regex

Indonesian mobile numbers follow these patterns:
- Domestic: `08[1-9]XX-XXXX-XXXX` (10-13 digits total)
- International without plus: `628[1-9]XX-XXXX-XXXX`
- International with plus: `+628[1-9]XX-XXXX-XXXX`

All major Indonesian operators use prefixes: 0811-0819, 0821-0823, 0827-0828, 0831-0833, 0838, 0851-0853, 0855-0859, 0877-0878, 0881-0889, 0895-0899.

**Regex:**
```
^(?:\+62|62|0)8[1-9]\d{7,11}$
```

This validates:
- `087812345678` (domestic, 12 digits)
- `6287812345678` (without +, 13 digits)
- `+6287812345678` (international, 14 chars)
- `08123456789` (shorter 11-digit number)
- `0812345678901` (longer 13-digit number)

### 3.2 Normalization Function

All formats are normalized to `+628xxxxxxxxxx` before storage:

```go
func NormalizeWhatsApp(input string) string {
    input = strings.TrimSpace(input)
    input = strings.ReplaceAll(input, "-", "")
    input = strings.ReplaceAll(input, " ", "")

    switch {
    case strings.HasPrefix(input, "+62"):
        return input
    case strings.HasPrefix(input, "62"):
        return "+" + input
    case strings.HasPrefix(input, "0"):
        return "+62" + input[1:]
    default:
        return input
    }
}
```

### 3.3 Where Normalization Happens

1. **Registration handler** — normalize before passing to usecase
2. **Login handler** — normalize identifier if it looks like a phone number before lookup
3. **Login usecase** — update identifier detection: if normalized number starts with `+628`, use WhatsApp lookup

### 3.4 Login Identifier Detection Update

Current logic: `@` in identifier → email, else → WhatsApp.

Updated logic:
1. If contains `@` → email lookup
2. If matches WhatsApp regex → normalize, then WhatsApp lookup
3. Else → treat as WhatsApp (backward compatible)

### 3.5 DTO Validation Update

**RegisterRequest** WhatsApp field:
```go
WhatsApp string `json:"whatsapp" validate:"required,whatsapp"`
```

Custom validator `whatsapp` registered with go-playground/validator using the regex above.

---

## 4. Audit Logger Wiring

The `AuditLogger` middleware is already implemented and tested in `internal/middleware/audit_logger.go`. It just needs to be wired into the middleware chain in `main.go`.

**Add after** the existing middleware stack, before route definitions:

```go
api.Use(middleware.AuditLogger(logger.Log))
```

This logs structured auth events (register/login/logout/refresh success/failure) with request_id, IP, user_agent, and duration.

---

## 5. Database Migration

Add new columns to `users` table via GORM AutoMigrate (existing approach):

```go
type UserModel struct {
    // ... existing fields ...
    Bio          string `gorm:"size:500"`
    AvatarURL    string `gorm:"size:500"`
    Address      string `gorm:"size:255"`
    City         string `gorm:"size:100"`
    Province     string `gorm:"size:100"`
    Skills       string `gorm:"size:500"`
    ServiceAreas string `gorm:"size:500"`
}
```

---

## 6. Frontend Changes

### 6.1 Auth Context

- Update `user` state shape to include new profile fields
- Call `authService.getMe()` after login to hydrate full profile data

### 6.2 Profile Page

- Replace hardcoded `profileService.js` stub with real `GET /api/v1/me` call
- Add edit profile form calling `PATCH /api/v1/me`
- Provider-specific fields shown/hidden based on role

### 6.3 Register Form

- WhatsApp validation regex updated to match backend: `^(?:\+62|62|0)8[1-9]\d{7,11}$`
- Normalize before sending to API

### 6.4 Login Form

- Accept all WhatsApp formats (08xx, 628xx, +628xx)
- Send as-is to backend (backend normalizes)

---

## 7. Files Changed

### Backend (new)
- `internal/handler/profile_handler.go`
- `internal/dto/profile_dto.go`
- `internal/usecase/profile_usecase.go`
- `pkg/whatsapp/whatsapp.go` (validation + normalization)
- `pkg/whatsapp/whatsapp_test.go`

### Backend (modified)
- `internal/domain/entity/user.go` — add new fields
- `internal/repository/model/user_model.go` — add new columns
- `internal/repository/gorm/user_repository.go` — add Update method
- `internal/domain/repository/user_repository.go` — add Update to interface
- `internal/dto/auth_dto.go` — update UserResponse, add custom validator
- `internal/handler/auth_handler.go` — normalize WhatsApp before usecase
- `internal/usecase/auth_usecase.go` — update login identifier detection
- `internal/middleware/audit_logger.go` — extend to cover /api/v1/me
- `cmd/server/main.go` — wire audit logger, register profile routes
- `pkg/validator/validator.go` — register custom whatsapp validator

### Frontend (modified)
- `src/services/auth.service.js` — add getMe and updateProfile calls
- `src/context/AuthContext.jsx` — hydrate full profile, add updateProfile
- `src/pages/profile/hooks/useProfile.js` — replace stub with real API
- `src/pages/profile/service/profileService.js` — replace stub with real API calls
- `src/components/RegisterForm.jsx` — update WhatsApp regex

# Service CRUD + Booking CRUD

**Date:** 2026-06-14
**Status:** Approved
**Scope:** Backend + Frontend

---

## 1. Overview

Implement the core marketplace domain: providers create service listings, customers book services. These two entities are tightly coupled — a booking references a specific service.

---

## 2. Domain Entities

### 2.1 Service

```go
// internal/domain/entity/service.go
type Service struct {
    ID              uint
    ProviderID      uint
    Title           string
    Category        string
    Description     string
    UnitType        string     // "per_session" | "per_hour"
    BasePrice       int64      // in Rupiah
    DurationMinutes int
    Images          []ServiceImage
    AddOns          []ServiceAddOn
    ServiceAreas    string     // comma-separated
    Status          string     // "draft" | "active" | "inactive"
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

### 2.2 ServiceImage

```go
type ServiceImage struct {
    ID        uint
    ServiceID uint
    URL       string
    SortOrder int
}
```

### 2.3 ServiceAddOn

```go
type ServiceAddOn struct {
    ID        uint
    ServiceID uint
    Name      string
    Price     int64   // in Rupiah
}
```

### 2.4 Booking

```go
type Booking struct {
    ID            uint
    CustomerID    uint
    ServiceID     uint
    ProviderID    uint          // denormalized from service for query efficiency
    ScheduleDate  time.Time
    ScheduleTime  string        // "09:00" format
    Address       string
    Notes         string
    SubTotal      int64
    PlatformFee   int64         // e.g., 5000
    Total         int64
    PaymentMethod string        // "cash_on_service" | "bank_transfer" | "e_wallet"
    Status        string        // see status flow
    BookingAddOns []BookingAddOn
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

### 2.5 BookingAddOn

```go
type BookingAddOn struct {
    ID        uint
    BookingID uint
    Name      string
    Price     int64
}
```

---

## 3. Categories

Matching frontend category data:

| Slug | Display Name |
|------|-------------|
| `cleaning` | Bersih-bersih |
| `handyman` | Tukang |
| `massage` | Pijat |
| `gardening` | Kebun |
| `ac_service` | AC |
| `moving` | Pindahan |
| `laundry` | Laundry |
| `shopping` | Belanja |
| `errands` | Serabutan |
| `other` | Lainnya |

---

## 4. Booking Status Flow

```
                  ┌─→ rejected (terminal)
                  │
customer creates ─→ pending ─→ accepted ─→ confirmed ─→ completed (terminal)
                  │             │            │
                  └─→ cancelled ←────────────┘
                     (terminal)
```

**Status meanings:**
- `pending` — Customer created the booking, awaiting provider response
- `accepted` — Provider accepted the booking request (first acknowledgment)
- `confirmed` — Provider confirms the job is done (tells platform work is finished)
- `completed` — Customer confirms the work was done satisfactorily
- `rejected` — Provider rejected the booking request (terminal)
- `cancelled` — Either party cancelled before confirmation (terminal)

**Transition rules:**
| From | Action | Who | To |
|------|--------|-----|-----|
| `pending` | accept | provider only | `accepted` |
| `pending` | reject | provider only | `rejected` |
| `pending` | cancel | customer or provider | `cancelled` |
| `accepted` | confirm | provider only | `confirmed` |
| `accepted` | cancel | customer or provider | `cancelled` |
| `confirmed` | complete | customer only | `completed` |

**Terminal states** (`rejected`, `cancelled`, `completed`) cannot transition.

**Review (future):** After a booking reaches `completed` status, the customer may optionally leave a review. This is out of scope for this spec but the status flow accommodates it.

---

## 5. API Endpoints

### 5.1 Services

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| `POST` | `/api/v1/services` | JWT | provider | Create service |
| `GET` | `/api/v1/services/:id` | None | — | Get service detail (public) |
| `PATCH` | `/api/v1/services/:id` | JWT | provider (owner) | Update service |
| `DELETE` | `/api/v1/services/:id` | JWT | provider (owner) | Set status to inactive |
| `GET` | `/api/v1/services` | None | — | List/search with filters |
| `GET` | `/api/v1/my-services` | JWT | provider | Provider's own services |

#### POST /api/v1/services

```json
{
  "title": "Deep Cleaning Rumah",
  "category": "cleaning",
  "description": "Bersih-bersih rumah menyeluruh...",
  "unit_type": "per_session",
  "base_price": 150000,
  "duration_minutes": 120,
  "images": [
    { "url": "/uploads/abc123_original.jpg", "sort_order": 0 },
    { "url": "/uploads/def456_original.jpg", "sort_order": 1 }
  ],
  "add_ons": [
    { "name": "Bawa alat kebersihan", "price": 25000 },
    { "name": "Extra kamar mandi", "price": 30000 }
  ],
  "service_areas": "Jakarta Selatan,Jakarta Pusat",
  "status": "active"
}
```

**Validation:**
- `title`: required, 3-100 chars
- `category`: required, one of defined categories
- `description`: required, 10-2000 chars
- `unit_type`: required, one of `per_session`, `per_hour`
- `base_price`: required, min 10000
- `duration_minutes`: required, min 15
- `images`: required, 1-8 items, URLs from upload endpoint
- `add_ons`: optional, max 10
- `service_areas`: required, max 500 chars
- `status`: optional, default `draft`

**Response (201):** Full service object with ID

#### GET /api/v1/services (List/Search)

**Query params:**

| Param | Type | Description |
|-------|------|-------------|
| `q` | string | Search title/description |
| `category` | string | Filter by category slug |
| `city` | string | Filter by provider city |
| `min_price` | int64 | Minimum base price |
| `max_price` | int64 | Maximum base price |
| `sort` | string | `newest`, `price_asc`, `price_desc` (default: `newest`) |
| `page` | int | Page number (default: 1) |
| `limit` | int | Items per page (default: 20, max: 50) |

**Response:**
```json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 150,
    "total_pages": 8
  }
}
```

#### GET /api/v1/services/:id

**Response (public, no WhatsApp):**
```json
{
  "data": {
    "id": 1,
    "provider": {
      "id": 5,
      "name": "Budi Santoso",
      "avatar_url": "/uploads/avatar_original.jpg",
      "city": "Jakarta",
      "province": "DKI Jakarta"
    },
    "title": "Deep Cleaning Rumah",
    "category": "cleaning",
    "description": "...",
    "unit_type": "per_session",
    "base_price": 150000,
    "duration_minutes": 120,
    "images": [
      { "url": "/uploads/abc_original.jpg", "sort_order": 0 }
    ],
    "add_ons": [
      { "id": 1, "name": "Bawa alat kebersihan", "price": 25000 }
    ],
    "service_areas": "Jakarta Selatan, Jakarta Pusat",
    "status": "active",
    "created_at": "2026-06-01T10:00:00Z"
  }
}
```

### 5.2 Bookings

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/bookings` | JWT | Create booking |
| `GET` | `/api/v1/bookings/:id` | JWT | Get booking detail (participants only) |
| `GET` | `/api/v1/my-bookings` | JWT | User's bookings |
| `PATCH` | `/api/v1/bookings/:id/accept` | JWT | Provider accepts |
| `PATCH` | `/api/v1/bookings/:id/reject` | JWT | Provider rejects |
| `PATCH` | `/api/v1/bookings/:id/complete` | JWT | Mark completed |
| `PATCH` | `/api/v1/bookings/:id/cancel` | JWT | Cancel booking |

#### POST /api/v1/bookings

```json
{
  "service_id": 1,
  "schedule_date": "2026-06-20",
  "schedule_time": "09:00",
  "address": "Jl. Sudirman No. 1, Jakarta Selatan",
  "notes": "Tolong bawa alat pel",
  "add_on_ids": [1, 2],
  "payment_method": "cash_on_service"
}
```

**Server-side calculations:**
- `sub_total` = service.base_price + sum of selected add-on prices
- `platform_fee` = 5000 (configurable)
- `total` = sub_total + platform_fee

**Validation:**
- `service_id`: required, must exist and be active
- `schedule_date`: required, must be in the future
- `schedule_time`: required, valid HH:MM format
- `address`: required, max 255 chars
- `notes`: optional, max 500 chars
- `add_on_ids`: optional, must belong to the service
- `payment_method`: required, one of defined methods

**Response (201):**
```json
{
  "data": {
    "id": 42,
    "customer": { "id": 3, "name": "Andi" },
    "service": { "id": 1, "title": "Deep Cleaning Rumah" },
    "provider": { "id": 5, "name": "Budi Santoso" },
    "schedule_date": "2026-06-20",
    "schedule_time": "09:00",
    "address": "Jl. Sudirman No. 1, Jakarta Selatan",
    "notes": "Tolong bawa alat pel",
    "add_ons": [
      { "name": "Bawa alat kebersihan", "price": 25000 },
      { "name": "Extra kamar mandi", "price": 30000 }
    ],
    "sub_total": 205000,
    "platform_fee": 5000,
    "total": 210000,
    "payment_method": "cash_on_service",
    "status": "pending",
    "created_at": "2026-06-14T15:30:00Z"
  }
}
```

#### GET /api/v1/my-bookings

**Query params:**

| Param | Type | Description |
|-------|------|-------------|
| `role` | string | `customer` or `provider` (default: auto-detect from user role) |
| `status` | string | Filter by status |
| `page` | int | Page number |
| `limit` | int | Items per page |

#### Status Transition Endpoints

All return the updated booking object. Authorization checks:
- `accept`: only service provider, only from `pending`
- `reject`: only service provider, only from `pending`
- `confirm`: only service provider, only from `accepted` (provider confirms job is done)
- `complete`: only customer, only from `confirmed` (customer confirms work done)
- `cancel`: customer or provider, only from `pending` or `accepted`

**Error responses for invalid transitions:**
- `400 INVALID_TRANSITION` — Status transition not allowed (e.g., trying to accept an already-completed booking)
- `403 FORBIDDEN` — User is not authorized to perform this action on this booking
- `404 NOT_FOUND` — Booking does not exist

---

## 6. Repository Interfaces

### 6.1 ServiceRepository

```go
type ServiceRepository interface {
    Create(ctx context.Context, service *entity.Service) error
    FindByID(ctx context.Context, id uint) (*entity.Service, error)
    Update(ctx context.Context, service *entity.Service) error
    List(ctx context.Context, filter ServiceFilter) ([]*entity.Service, int64, error)
    FindByProviderID(ctx context.Context, providerID uint, filter ServiceFilter) ([]*entity.Service, int64, error)
    SetStatus(ctx context.Context, id uint, status string) error
}

type ServiceFilter struct {
    Query    string
    Category string
    City     string
    MinPrice int64
    MaxPrice int64
    Sort     string
    Page     int
    Limit    int
}
```

### 6.2 BookingRepository

```go
type BookingRepository interface {
    Create(ctx context.Context, booking *entity.Booking) error
    FindByID(ctx context.Context, id uint) (*entity.Booking, error)
    UpdateStatus(ctx context.Context, id uint, status string) error
    FindByUserID(ctx context.Context, userID uint, role string, filter BookingFilter) ([]*entity.Booking, int64, error)
}

type BookingFilter struct {
    Status string
    Page   int
    Limit  int
}
```

---

## 7. Persistence Models

### 7.1 ServiceModel

```go
type ServiceModel struct {
    ID              uint           `gorm:"primaryKey"`
    ProviderID      uint           `gorm:"index;not null"`
    Title           string         `gorm:"size:100;not null"`
    Category        string         `gorm:"size:50;index;not null"`
    Description     string         `gorm:"size:2000;not null"`
    UnitType        string         `gorm:"size:20;not null"`
    BasePrice       int64          `gorm:"not null"`
    DurationMinutes int            `gorm:"not null"`
    ServiceAreas    string         `gorm:"size:500;not null"`
    Status          string         `gorm:"size:20;default:draft;index;not null"`
    Images          []ServiceImageModel `gorm:"foreignKey:ServiceID"`
    AddOns          []ServiceAddOnModel `gorm:"foreignKey:ServiceID"`
    CreatedAt       time.Time
    UpdatedAt       time.Time
    DeletedAt       gorm.DeletedAt `gorm:"index"`
}
```

### 7.2 ServiceImageModel

```go
type ServiceImageModel struct {
    ID        uint   `gorm:"primaryKey"`
    ServiceID uint   `gorm:"index;not null"`
    URL       string `gorm:"size:500;not null"`
    SortOrder int    `gorm:"default:0"`
}
```

### 7.3 ServiceAddOnModel

```go
type ServiceAddOnModel struct {
    ID        uint   `gorm:"primaryKey"`
    ServiceID uint   `gorm:"index;not null"`
    Name      string `gorm:"size:100;not null"`
    Price     int64  `gorm:"not null"`
}
```

### 7.4 BookingModel

```go
type BookingModel struct {
    ID            uint           `gorm:"primaryKey"`
    CustomerID    uint           `gorm:"index;not null"`
    ServiceID     uint           `gorm:"index;not null"`
    ProviderID    uint           `gorm:"index;not null"`
    ScheduleDate  time.Time      `gorm:"not null"`
    ScheduleTime  string         `gorm:"size:5;not null"`
    Address       string         `gorm:"size:255;not null"`
    Notes         string         `gorm:"size:500"`
    SubTotal      int64          `gorm:"not null"`
    PlatformFee   int64          `gorm:"not null"`
    Total         int64          `gorm:"not null"`
    PaymentMethod string         `gorm:"size:30;not null"`
    Status        string         `gorm:"size:20;default:pending;index;not null"`
    BookingAddOns []BookingAddOnModel `gorm:"foreignKey:BookingID"`
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

### 7.5 BookingAddOnModel

```go
type BookingAddOnModel struct {
    ID        uint   `gorm:"primaryKey"`
    BookingID uint   `gorm:"index;not null"`
    Name      string `gorm:"size:100;not null"`
    Price     int64  `gorm:"not null"`
}
```

---

## 8. Configuration Additions

```env
# Booking
PLATFORM_FEE=5000
```

---

## 9. Domain Errors Added

```go
// internal/domain/error/errors.go additions
ErrServiceNotFound    = errors.New("service not found")
ErrServiceInactive    = errors.New("service is not active")
ErrBookingNotFound    = errors.New("booking not found")
ErrInvalidTransition  = errors.New("invalid status transition")
ErrNotServiceOwner    = errors.New("user is not the service owner")
ErrNotBookingParticipant = errors.New("user is not a booking participant")
```

---

## 10. Transaction Boundaries

### Service Creation
Single transaction: create service + images + add-ons atomically.

### Booking Creation
Single transaction: create booking + booking add-ons atomically. Also validates service exists and is active within the same transaction.

### Status Transitions
Single update with optimistic status check (WHERE status = expected_status).

---

## 11. Files Created

### Backend (new)
- `internal/domain/entity/service.go`
- `internal/domain/entity/booking.go`
- `internal/domain/repository/service_repository.go`
- `internal/domain/repository/booking_repository.go`
- `internal/repository/model/service_model.go`
- `internal/repository/model/booking_model.go`
- `internal/repository/gorm/service_repository.go`
- `internal/repository/gorm/booking_repository.go`
- `internal/usecase/service_usecase.go`
- `internal/usecase/booking_usecase.go`
- `internal/handler/service_handler.go`
- `internal/handler/booking_handler.go`
- `internal/dto/service_dto.go`
- `internal/dto/booking_dto.go`

### Backend (modified)
- `internal/infrastructure/config/config.go` — add PlatformFee config
- `cmd/server/main.go` — wire service/booking deps, register routes, add AutoMigrate for new models
- `.env.example` — add PLATFORM_FEE

### Frontend (modified)
- `src/pages/create-service/service/createService.js` — connect to real API
- `src/pages/list-job/service/listService.js` — connect to real API (or create new)
- `src/pages/job-detail/service/detailService.js` — connect to real API
- `src/pages/booking-schedule/service/scheduleService.js` — connect to real API
- `src/pages/booking-confirm/service/confirmService.js` — connect to real API
- `src/pages/create-job/service/createJobService.js` — this is job posting (future, not in this spec)

# Image Upload with Storage Abstraction

**Date:** 2026-06-14
**Status:** Approved
**Scope:** Backend

---

## 1. Overview

A pluggable image upload system with automatic compression and variant generation. Storage backend is configurable via a single env var (`local`, `s3`, or `minio`). Uses `github.com/disintegration/imaging` for image processing.

---

## 2. Architecture

### 2.1 Layer Structure

```
handler/upload_handler.go        — HTTP: parse multipart, validate, respond with URLs
usecase/upload_usecase.go       — Orchestrate: validate → process → store → return URLs
domain/service/image_processor.go — Interface + impl: decode, resize, compress
domain/repository/storage_repository.go — Interface: Upload, Delete, GetURL
repository/local/storage.go     — Local filesystem implementation
repository/s3/storage.go        — S3/MinIO implementation (via minio-go)
```

### 2.2 Storage Interface (Domain Layer)

```go
// internal/domain/repository/storage_repository.go
package repository

import "context"

type StorageRepository interface {
    Upload(ctx context.Context, key string, data []byte, contentType string) (string, error)
    Delete(ctx context.Context, key string) error
    GetURL(key string) string
}
```

- `Upload` stores data at the given key, returns the accessible URL
- `Delete` removes a stored object by key
- `GetURL` returns the public URL for a given key (no network call)

### 2.3 Image Processor Interface (Domain Layer)

```go
// internal/domain/service/image_processor.go
package service

type ImageProcessor interface {
    Process(data []byte, opts ProcessOptions) ([]ProcessedImage, error)
}

type ProcessOptions struct {
    Variants []VariantConfig
    Quality  int  // JPEG quality 1-100, default 85
}

type VariantConfig struct {
    Name       string  // "thumbnail", "medium", "original"
    MaxWidth   int     // 0 = keep original width
    MaxHeight  int     // 0 = keep aspect ratio
}

type ProcessedImage struct {
    Name        string
    Data        []byte
    ContentType string
    Width       int
    Height      int
}
```

### 2.4 Image Processor Implementation

Uses `github.com/disintegration/imaging`:

```go
// internal/domain/service/imaging_processor.go
type ImagingProcessor struct {
    defaultQuality int
}

func (p *ImagingProcessor) Process(data []byte, opts ProcessOptions) ([]ProcessedImage, error) {
    // 1. Decode with auto-orientation (EXIF)
    img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))

    // 2. For each variant:
    //    - Resize using imaging.Fit() or imaging.Resize() with Lanczos filter
    //    - Encode as JPEG with configured quality
    //    - Return ProcessedImage

    // 3. "original" variant: compress only (no resize) at configured quality
}
```

**Default variant configuration:**

| Variant | Max Width | Purpose |
|---------|-----------|---------|
| `thumbnail` | 200px | Listing cards, avatars |
| `medium` | 800px | Detail pages |
| `original` | 0 (no resize) | Full view, compressed at 85% quality |

---

## 3. Configuration

### 3.1 Environment Variables

```env
# Storage driver: "local", "s3", or "minio"
STORAGE_DRIVER=local

# Local storage settings
STORAGE_LOCAL_PATH=./uploads
STORAGE_LOCAL_URL=/uploads

# S3/MinIO settings (shared)
STORAGE_S3_ENDPOINT=s3.amazonaws.com
STORAGE_S3_BUCKET=serabutin-uploads
STORAGE_S3_REGION=ap-southeast-1
STORAGE_S3_ACCESS_KEY=
STORAGE_S3_SECRET_KEY=
STORAGE_S3_USE_SSL=true
STORAGE_S3_URL_PREFIX=
```

### 3.2 Driver Selection

A factory function in `internal/infrastructure/storage/factory.go`:

```go
func NewStorageRepository(cfg *config.Config) (repository.StorageRepository, error) {
    switch cfg.StorageDriver {
    case "local":
        return local.NewLocalStorage(cfg.StorageLocalPath, cfg.StorageLocalURL)
    case "s3", "minio":
        return s3.NewS3Storage(cfg.StorageS3Endpoint, cfg.StorageS3Bucket, ...)
    default:
        return nil, fmt.Errorf("unknown storage driver: %s", cfg.StorageDriver)
    }
}
```

Both `s3` and `minio` use the same `minio-go` client since MinIO is S3-compatible. The difference is just endpoint configuration.

---

## 4. API Endpoint

### POST /api/v1/upload

- **Auth:** JWT required
- **Content-Type:** `multipart/form-data`
- **Field:** `images` (multiple files allowed)
- **Limits:** Max 8 files, 10MB each, JPEG/PNG/WebP only

**Request:**
```
POST /api/v1/upload
Authorization: Bearer <token>
Content-Type: multipart/form-data

images: [file1.jpg, file2.png]
```

**Response (201):**
```json
{
  "data": [
    {
      "key": "a1b2c3d4",
      "original_url": "/uploads/a1b2c3d4_original.jpg",
      "medium_url": "/uploads/a1b2c3d4_medium.jpg",
      "thumbnail_url": "/uploads/a1b2c3d4_thumb.jpg"
    },
    {
      "key": "e5f6g7h8",
      "original_url": "/uploads/e5f6g7h8_original.jpg",
      "medium_url": "/uploads/e5f6g7h8_medium.jpg",
      "thumbnail_url": "/uploads/e5f6g7h8_thumb.jpg"
    }
  ]
}
```

**Error responses:**
- `400` — No files, too many files, file too large, unsupported format
- `401` — Not authenticated
- `500` — Processing or storage error

---

## 5. File Naming Convention

```
{uuid}_{variant}.{ext}
```

- UUID: v4 random (collision-free)
- Variant: `original`, `medium`, `thumb`
- Extension: always `.jpg` (all variants encoded as JPEG for consistency)

Example: `a1b2c3d4-e5f6-7890-abcd-ef1234567890_original.jpg`

---

## 6. Storage Implementations

### 6.1 Local Storage

```go
type LocalStorage struct {
    basePath string  // e.g., "./uploads"
    baseURL  string  // e.g., "/uploads"
}
```

- Creates directories on init if they don't exist
- `Upload`: writes file to `basePath/key`
- `GetURL`: returns `baseURL/key`
- Files served by nginx/reverse proxy from the `basePath` directory

### 6.2 S3/MinIO Storage

```go
type S3Storage struct {
    client *minio.Client
    bucket string
    urlPrefix string
}
```

- Uses `github.com/minio/minio-go/v7`
- `Upload`: `client.PutObject()` with content type
- `GetURL`: returns `urlPrefix/bucket/key` (assumes public read policy) or generates presigned URL
- Bucket auto-created on init if it doesn't exist

---

## 7. Upload Flow

```
1. Handler receives multipart form
2. Handler validates: file count ≤ 8, size ≤ 10MB, content type ∈ {jpeg, png, webp}
3. Handler calls usecase.Upload(ctx, files)
4. Usecase for each file:
   a. Generate UUID key
   b. Call ImageProcessor.Process(data, defaultOpts)
   c. For each processed variant:
      - Call StorageRepository.Upload(ctx, "{key}_{variant}.jpg", data, "image/jpeg")
   d. Collect URLs
5. Return all URLs grouped by key
```

---

## 8. Body Limit Update

Current `BodyLimit` in `main.go` is 1MB. Must increase for image uploads:

```go
// Global body limit for regular endpoints
BodyLimit: 1 * 1024 * 1024,  // 1MB

// Upload endpoint needs higher limit — handled via route-level limit
app.Post("/api/v1/upload", func(c fiber.Ctx) error { ... })
```

Or set global to 50MB and rely on handler-level validation for non-upload endpoints.

---

## 9. Dependencies Added

```
github.com/disintegration/imaging  v1.6.2  — image processing
github.com/minio/minio-go/v7       latest  — S3/MinIO storage
github.com/google/uuid             (already in go.mod) — file naming
```

---

## 10. Files Created

### Backend (new)
- `internal/handler/upload_handler.go`
- `internal/usecase/upload_usecase.go`
- `internal/dto/upload_dto.go`
- `internal/domain/service/image_processor.go` (interface)
- `internal/domain/service/imaging_processor.go` (implementation)
- `internal/domain/service/imaging_processor_test.go`
- `internal/domain/repository/storage_repository.go` (interface)
- `internal/repository/local/storage.go`
- `internal/repository/local/storage_test.go`
- `internal/repository/s3/storage.go`
- `internal/repository/s3/storage_test.go`
- `internal/infrastructure/storage/factory.go`

### Backend (modified)
- `internal/infrastructure/config/config.go` — add storage config fields
- `cmd/server/main.go` — wire storage, image processor, upload routes
- `.env.example` — add storage env vars
- `go.mod` / `go.sum` — add imaging + minio-go deps
- `docker-compose.yml` — mount uploads volume for local storage

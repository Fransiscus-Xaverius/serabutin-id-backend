# Payout Bank Details (Mitra) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every Mitra can store an encrypted bank destination (bank code, account holder name, account number) so DOKU can later pay them directly from serabutin's hub sub-account, with the encryption key held only in the environment.

**Architecture:** Clean Architecture, following the existing repo layout. A new `payout_accounts` table holds one destination per user; the account number and holder name are encrypted with AES-256-GCM by a small infrastructure package; a `PayoutUsecase` is the only place that decrypts; a `PayoutHandler` exposes masked reads and writes to the owning Mitra only. The key comes from the environment and the server refuses to boot without it.

**Tech Stack:** Go 1.25, GoFiber v3, GORM/Postgres, testify, testcontainers-go; React 19 + Vite + Vitest + Testing Library on the frontend.

**Spec:** `docs/superpowers/specs/2026-10-28-payout-bank-details-design.md`

## Global Constraints

- The encryption key is `PAYOUT_ENCRYPTION_KEY`: base64 of exactly 32 bytes, from the environment only. Never commit it, never log it, never return it.
- A missing or malformed key is a **fatal startup error**. No plaintext fallback, ever.
- The account number and the account holder name are the only encrypted fields. `bank_code`, `bank_name`, `account_last4` and the fingerprint stay in plaintext.
- No API response, no log line and no notification may contain the account number or the holder name.
- `status` starts at `unverified`: DOKU account validation is not built yet, so a saved destination is not a validated one.
- Only a signed-in user with role `provider` may read or write their payout destination.
- Go files: `gofmt` clean. Tests: `go test ./...` and `npm run test:run` green before each commit.
- Do not touch `serabutin-technical-documentation` or `test-screenshots` in the code tasks; Task 7 does the documentation on purpose.

## Review Focus

The inputs most likely to bite someone using this, and the behaviour each must produce:

1. **The key is absent, malformed, or the wrong length** → the server must refuse to start with an actionable message, not fall back to plaintext.
2. **The same account number is submitted twice, or by two Mitras** → the second write must be detected as a duplicate through the fingerprint without decrypting anything.
3. **The account number is submitted, then read back** → the API must return only `account_last4`; the plaintext must not appear in the response, in a log, or in an error message.
4. **A customer (role `customer`) calls the payout endpoints** → 403, and no row is created.
5. **The holder name does not match the registered account name** → stored with `name_mismatch = true` and `status = unverified`; the request must not fail and the name must not be silently rewritten.

---

### Task 1: Encryption primitive

**Files:**
- Create: `internal/infrastructure/encryption/cipher.go`
- Test: `internal/infrastructure/encryption/cipher_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `encryption.NewCipher(key []byte, keyID string) (*Cipher, error)`, `(*Cipher).Encrypt(plaintext string) (string, error)`, `(*Cipher).Decrypt(encoded string) (string, error)`, `(*Cipher).Fingerprint(plaintext string) string`, `(*Cipher).KeyID() string`, `encryption.ErrInvalidKeySize`.

- [ ] **Step 1: Write the failing test**

Create `internal/infrastructure/encryption/cipher_test.go`:

```go
package encryption

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}

func TestCipher_RoundTrip(t *testing.T) {
	c, err := NewCipher(testKey(), "v1")
	require.NoError(t, err)

	encoded, err := c.Encrypt("1234567890")
	require.NoError(t, err)
	assert.NotContains(t, encoded, "1234567890")

	plain, err := c.Decrypt(encoded)
	require.NoError(t, err)
	assert.Equal(t, "1234567890", plain)
}

func TestCipher_NonceIsUniquePerValue(t *testing.T) {
	c, err := NewCipher(testKey(), "v1")
	require.NoError(t, err)

	first, err := c.Encrypt("1234567890")
	require.NoError(t, err)
	second, err := c.Encrypt("1234567890")
	require.NoError(t, err)
	assert.NotEqual(t, first, second, "the same value must not encrypt to the same ciphertext")
}

func TestCipher_WrongKeyCannotDecrypt(t *testing.T) {
	c, err := NewCipher(testKey(), "v1")
	require.NoError(t, err)
	encoded, err := c.Encrypt("secret")
	require.NoError(t, err)

	other, err := NewCipher(append(testKey(), 0)[:32], "v1")
	require.NoError(t, err)
	otherKey := make([]byte, 32)
	for i := range otherKey {
		otherKey[i] = byte(100 + i)
	}
	other, err = NewCipher(otherKey, "v1")
	require.NoError(t, err)

	_, err = other.Decrypt(encoded)
	assert.Error(t, err)
}

func TestCipher_TamperedCiphertextFails(t *testing.T) {
	c, err := NewCipher(testKey(), "v1")
	require.NoError(t, err)
	encoded, err := c.Encrypt("1234567890")
	require.NoError(t, err)

	tampered := []byte(encoded)
	tampered[len(tampered)-2] = 'A'
	if tampered[len(tampered)-2] == 'A' {
		tampered[len(tampered)-2] = 'B'
	}
	_, err = c.Decrypt(string(tampered))
	assert.Error(t, err)
}

func TestCipher_FingerprintIsStableAndOpaque(t *testing.T) {
	c, err := NewCipher(testKey(), "v1")
	require.NoError(t, err)

	first := c.Fingerprint("1234567890")
	second := c.Fingerprint("1234567890")
	assert.Equal(t, first, second)
	assert.NotEqual(t, first, c.Fingerprint("9876543210"))
	assert.NotContains(t, first, "1234567890")
	assert.Len(t, first, 32)
}

func TestNewCipher_RejectsWrongKeySize(t *testing.T) {
	_, err := NewCipher(make([]byte, 16), "v1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidKeySize)

	_, err = NewCipher(strings.Repeat("x", 32), "v1")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/infrastructure/encryption/...`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

Create `internal/infrastructure/encryption/cipher.go`:

```go
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	// KeySize is the AES-256 key length in bytes.
	KeySize = 32
	// fingerprintPrefix domain-separates the fingerprint from any other
	// use of the same key.
	fingerprintPrefix = "payout-account:v1:"
)

var ErrInvalidKeySize = errors.New("encryption key must be 32 bytes")

// Cipher encrypts short secrets with AES-256-GCM. The same value never
// encrypts to the same ciphertext (random nonce per call) and a modified
// ciphertext fails to decrypt.
type Cipher struct {
	aead  cipher.AEAD
	key   []byte
	keyID string
}

func NewCipher(key []byte, keyID string) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("%w: got %d bytes", ErrInvalidKeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	if keyID == "" {
		keyID = "v1"
	}
	return &Cipher{aead: aead, key: append([]byte(nil), key...), keyID: keyID}, nil
}

func (c *Cipher) KeyID() string { return c.keyID }

func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

func (c *Cipher) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(raw) < c.aead.NonceSize() {
		return "", errors.New("ciphertext is too short")
	}
	nonce, payload := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

// Fingerprint is a keyed digest of a value: it identifies an account number
// for duplicate detection without revealing it.
func (c *Cipher) Fingerprint(plaintext string) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(fingerprintPrefix + plaintext))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/infrastructure/encryption/... -count=1`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/infrastructure/encryption/
git add internal/infrastructure/encryption/
git commit -m "feat(payout): add AES-256-GCM cipher for payout secrets"
```

---

### Task 2: Environment key, parsed and required

**Files:**
- Modify: `internal/infrastructure/config/config.go`
- Modify: `cmd/server/main.go`
- Modify: `.env.example`
- Modify: `docker-compose.yml`
- Test: `internal/infrastructure/config/payment_config_test.go`

**Interfaces:**
- Consumes: `encryption.KeySize` (32).
- Produces: `config.DecodePayoutEncryptionKey(encoded string) ([]byte, error)`, `Config.PayoutEncryptionKey []byte`, `Config.PayoutEncryptionKeyID string`.

- [ ] **Step 1: Write the failing test**

Append to `internal/infrastructure/config/payment_config_test.go`:

```go
func TestDecodePayoutEncryptionKey_RejectsMissingMalformedAndWrongLength(t *testing.T) {
	_, err := DecodePayoutEncryptionKey("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PAYOUT_ENCRYPTION_KEY")

	_, err = DecodePayoutEncryptionKey("not base64!!")
	require.Error(t, err)

	_, err = DecodePayoutEncryptionKey(base64.StdEncoding.EncodeToString(make([]byte, 16)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestDecodePayoutEncryptionKey_AcceptsThirtyTwoBytes(t *testing.T) {
	key := make([]byte, 32)
	key[0] = 9

	decoded, err := DecodePayoutEncryptionKey(base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	assert.Len(t, decoded, 32)
	assert.Equal(t, byte(9), decoded[0])
}
```

Add `"encoding/base64"` to the test file imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/infrastructure/config/ -run TestDecodePayoutEncryptionKey`
Expected: FAIL — `DecodePayoutEncryptionKey` undefined.

- [ ] **Step 3: Write the implementation**

In `internal/infrastructure/config/config.go`:

```go
// Config fields to add:
//   PayoutEncryptionKey   []byte
//   PayoutEncryptionKeyID string
```

```go
	viper.SetDefault("PAYOUT_ENCRYPTION_KEY", "")
	viper.SetDefault("PAYOUT_ENCRYPTION_KEY_ID", "v1")
```

```go
// DecodePayoutEncryptionKey decodes the base64 encryption key from the
// environment. The key is required: there is no plaintext fallback, so a
// missing or malformed key stops the server instead of silently storing
// unprotected account numbers.
func DecodePayoutEncryptionKey(encoded string) ([]byte, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, fmt.Errorf("PAYOUT_ENCRYPTION_KEY is required (generate one with: openssl rand -base64 32)")
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("PAYOUT_ENCRYPTION_KEY must be base64: %w", err)
	}
	if len(decoded) != encryption.KeySize {
		return nil, fmt.Errorf("PAYOUT_ENCRYPTION_KEY must decode to %d bytes, got %d", encryption.KeySize, len(decoded))
	}
	return decoded, nil
}
```

In `Load()`, after the existing secret validation:

```go
	key, err := DecodePayoutEncryptionKey(viper.GetString("PAYOUT_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	cfg.PayoutEncryptionKey = key
	cfg.PayoutEncryptionKeyID = strings.TrimSpace(viper.GetString("PAYOUT_ENCRYPTION_KEY_ID"))
	if cfg.PayoutEncryptionKeyID == "" {
		cfg.PayoutEncryptionKeyID = "v1"
	}
```

Add the import `"github.com/serabutin/serabutin-id-backend/internal/infrastructure/encryption"`.

In `cmd/server/main.go`, right after `cfg, err := config.Load()` succeeds, add a fail-closed check so a future relaxation of `Load()` still cannot boot without a key:

```go
	if len(cfg.PayoutEncryptionKey) != encryption.KeySize {
		logger.Log.Fatal().Msg("PAYOUT_ENCRYPTION_KEY is missing or invalid; refusing to start")
	}
```

In `.env.example`:

```
# Encryption key for Mitra payout bank details (AES-256-GCM). REQUIRED:
# the server refuses to start without it. Generate with: openssl rand -base64 32
# Never commit a real key.
PAYOUT_ENCRYPTION_KEY=
# Which key encrypted a row; bump when rotating.
PAYOUT_ENCRYPTION_KEY_ID=v1
```

In `docker-compose.yml`, in the `api` service environment:

```yaml
      PAYOUT_ENCRYPTION_KEY: ${PAYOUT_ENCRYPTION_KEY:?set PAYOUT_ENCRYPTION_KEY (openssl rand -base64 32)}
      PAYOUT_ENCRYPTION_KEY_ID: ${PAYOUT_ENCRYPTION_KEY_ID:-v1}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/infrastructure/config/ -count=1`
Expected: PASS.

Then generate a local key and add it to the git-ignored `.env`:

```bash
KEY=$(openssl rand -base64 32)
printf '\n# Payout bank details encryption (local only, never commit)\nPAYOUT_ENCRYPTION_KEY=%s\nPAYOUT_ENCRYPTION_KEY_ID=v1\n' "$KEY" >> .env
```

- [ ] **Step 5: Commit**

```bash
git add internal/infrastructure/config/config.go cmd/server/main.go .env.example docker-compose.yml
git commit -m "feat(payout): require the payout encryption key from the environment"
```

Note: `.env` is git-ignored and must not be staged. Verify with `git status --short` before committing.

---

### Task 3: Payout account table and repository

**Files:**
- Create: `internal/domain/entity/payout_account.go`
- Create: `internal/repository/model/payout_account_model.go`
- Create: `internal/domain/repository/payout_account_repository.go`
- Create: `internal/repository/gorm/payout_account_repository.go`
- Create: `internal/repository/gorm/payout_account_live_test.go`
- Modify: `internal/repository/gorm/bounty_fixes_live_test.go` (add the payout model to the shared live-test migration)
- Modify: `cmd/server/main.go` (AutoMigrate)

**Interfaces:**
- Consumes: nothing.
- Produces: `entity.PayoutAccount`, `entity.PayoutAccountStatus*` constants, `repository.PayoutAccountRepository` with `Upsert(ctx, *entity.PayoutAccount) error`, `FindByUserID(ctx, userID uint) (*entity.PayoutAccount, error)`, `FindByFingerprint(ctx, fingerprint string) (*entity.PayoutAccount, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/repository/gorm/payout_account_live_test.go`. It reuses the package's shared live Postgres helper `setupBountyLiveDB(t)` (defined in `bounty_fixes_live_test.go`), so this test file adds no second container bootstrap. Add `&model.PayoutAccountModel{}` to that helper's `AutoMigrate` list.

```go
package gormrepo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

func TestLivePayoutAccount_UpsertAndFind(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	repo := NewPayoutAccountRepository(db)

	account := &entity.PayoutAccount{
		UserID:                    71,
		BankCode:                  "BCAIDJA",
		BankName:                  "Bank Central Asia",
		AccountHolderNameEncrypted: "enc-holder",
		AccountNumberEncrypted:    "enc-number",
		AccountLast4:              "7890",
		AccountFingerprint:        "fp-1",
		EncryptionKeyID:           "v1",
		Status:                    entity.PayoutAccountStatusUnverified,
	}
	require.NoError(t, repo.Upsert(ctx, account))
	assert.NotZero(t, account.ID)

	found, err := repo.FindByUserID(ctx, 71)
	require.NoError(t, err)
	assert.Equal(t, "BCAIDJA", found.BankCode)
	assert.Equal(t, "7890", found.AccountLast4)

	// Upsert replaces the row for the same user instead of adding one.
	account.BankCode = "MANDIRI"
	account.AccountFingerprint = "fp-2"
	require.NoError(t, repo.Upsert(ctx, account))

	found, err = repo.FindByUserID(ctx, 71)
	require.NoError(t, err)
	assert.Equal(t, "MANDIRI", found.BankCode)

	byFingerprint, err := repo.FindByFingerprint(ctx, "fp-2")
	require.NoError(t, err)
	assert.Equal(t, uint(71), byFingerprint.UserID)
}

func TestLivePayoutAccount_FindByUserID_Missing(t *testing.T) {
	db := setupBountyLiveDB(t)
	repo := NewPayoutAccountRepository(db)

	_, err := repo.FindByUserID(context.Background(), 999)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/repository/gorm/ -run TestLivePayoutAccount`
Expected: FAIL — types undefined.

- [ ] **Step 3: Write the implementation**

`internal/domain/entity/payout_account.go`:

```go
package entity

import "time"

const (
	PayoutAccountStatusUnverified = "unverified"
	PayoutAccountStatusVerified   = "verified"
	PayoutAccountStatusFailed     = "failed"
)

// PayoutAccount is the Mitra's bank destination for DOKU payouts. The
// account number and the holder name are encrypted; the bank code, the bank
// name, the last four digits and the fingerprint are not.
type PayoutAccount struct {
	ID                     uint
	UserID                 uint
	BankCode               string
	BankName               string
	AccountHolderNameEncrypted string
	AccountNumberEncrypted string
	AccountLast4           string
	AccountFingerprint     string
	EncryptionKeyID        string
	Status                 string
	NameMismatch           bool
	ValidatedAt            *time.Time
	LastError              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}
```

`internal/repository/model/payout_account_model.go`:

```go
package model

import "time"

type PayoutAccountModel struct {
	ID                        uint   `gorm:"primaryKey"`
	UserID                    uint   `gorm:"uniqueIndex;not null"`
	BankCode                  string `gorm:"size:16;not null"`
	BankName                  string `gorm:"size:100;not null"`
	AccountHolderNameEncrypted string `gorm:"type:text;not null"`
	AccountNumberEncrypted    string `gorm:"type:text;not null"`
	AccountLast4              string `gorm:"size:4;not null"`
	AccountFingerprint        string `gorm:"size:64;index"`
	EncryptionKeyID           string `gorm:"size:16;not null"`
	Status                    string `gorm:"size:20;not null;index"`
	NameMismatch              bool   `gorm:"not null;default:false"`
	ValidatedAt               *time.Time
	LastError                 string `gorm:"size:255"`
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

func (PayoutAccountModel) TableName() string { return "payout_accounts" }
```

`internal/domain/repository/payout_account_repository.go`:

```go
package repository

import (
	"context"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type PayoutAccountRepository interface {
	// Upsert creates or replaces the payout destination of a user.
	Upsert(ctx context.Context, account *entity.PayoutAccount) error
	FindByUserID(ctx context.Context, userID uint) (*entity.PayoutAccount, error)
	// FindByFingerprint locates a destination by its keyed digest, so a
	// duplicate account can be detected without decrypting.
	FindByFingerprint(ctx context.Context, fingerprint string) (*entity.PayoutAccount, error)
}
```

`internal/repository/gorm/payout_account_repository.go`:

```go
package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PayoutAccountRepository struct {
	db *gorm.DB
}

func NewPayoutAccountRepository(db *gorm.DB) *PayoutAccountRepository {
	return &PayoutAccountRepository{db: db}
}

func (r *PayoutAccountRepository) Upsert(ctx context.Context, account *entity.PayoutAccount) error {
	m := toPayoutAccountModel(account)
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"bank_code", "bank_name", "account_holder_name_encrypted",
			"account_number_encrypted", "account_last4", "account_fingerprint",
			"encryption_key_id", "status", "name_mismatch", "validated_at",
			"last_error", "updated_at",
		}),
	}).Create(m).Error
	if err != nil {
		return payoutAccountDBError(err)
	}
	account.ID = m.ID
	account.CreatedAt = m.CreatedAt
	account.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *PayoutAccountRepository) FindByUserID(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	var m model.PayoutAccountModel
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, payoutAccountNotFoundError()
		}
		return nil, payoutAccountDBError(err)
	}
	return toPayoutAccountEntity(&m), nil
}

func (r *PayoutAccountRepository) FindByFingerprint(ctx context.Context, fingerprint string) (*entity.PayoutAccount, error) {
	var m model.PayoutAccountModel
	err := r.db.WithContext(ctx).Where("account_fingerprint = ?", fingerprint).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, payoutAccountNotFoundError()
		}
		return nil, payoutAccountDBError(err)
	}
	return toPayoutAccountEntity(&m), nil
}

func toPayoutAccountModel(a *entity.PayoutAccount) *model.PayoutAccountModel {
	return &model.PayoutAccountModel{
		ID:                        a.ID,
		UserID:                    a.UserID,
		BankCode:                  a.BankCode,
		BankName:                  a.BankName,
		AccountHolderNameEncrypted: a.AccountHolderNameEncrypted,
		AccountNumberEncrypted:    a.AccountNumberEncrypted,
		AccountLast4:              a.AccountLast4,
		AccountFingerprint:        a.AccountFingerprint,
		EncryptionKeyID:           a.EncryptionKeyID,
		Status:                    a.Status,
		NameMismatch:              a.NameMismatch,
		ValidatedAt:               a.ValidatedAt,
		LastError:                 a.LastError,
		CreatedAt:                 a.CreatedAt,
		UpdatedAt:                 a.UpdatedAt,
	}
}

func toPayoutAccountEntity(m *model.PayoutAccountModel) *entity.PayoutAccount {
	return &entity.PayoutAccount{
		ID:                        m.ID,
		UserID:                    m.UserID,
		BankCode:                  m.BankCode,
		BankName:                  m.BankName,
		AccountHolderNameEncrypted: m.AccountHolderNameEncrypted,
		AccountNumberEncrypted:    m.AccountNumberEncrypted,
		AccountLast4:              m.AccountLast4,
		AccountFingerprint:        m.AccountFingerprint,
		EncryptionKeyID:           m.EncryptionKeyID,
		Status:                    m.Status,
		NameMismatch:              m.NameMismatch,
		ValidatedAt:               m.ValidatedAt,
		LastError:                 m.LastError,
		CreatedAt:                 m.CreatedAt,
		UpdatedAt:                 m.UpdatedAt,
	}
}

func payoutAccountNotFoundError() error {
	return apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_ACCOUNT_NOT_FOUND", "payout account not found")
}

func payoutAccountDBError(err error) error {
	return apperror.NewDomainError(apperror.ErrInternalServer, "DB_ERROR", err.Error())
}
```

In `internal/repository/gorm/bounty_fixes_live_test.go`, add the payout model to the shared migration:

```go
	require.NoError(t, db.AutoMigrate(
		&model.BookingModel{}, &model.BookingAddOnModel{}, &model.PaymentModel{}, &model.CancellationModel{},
		&model.ServiceModel{}, &model.BroadcastModel{}, &model.PayoutAccountModel{},
	))
```

In `cmd/server/main.go`, add `&model.PayoutAccountModel{}` to the `db.AutoMigrate(...)` list.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/repository/gorm/ -run TestLivePayoutAccount -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/domain/entity/payout_account.go internal/repository/model/payout_account_model.go internal/domain/repository/payout_account_repository.go internal/repository/gorm/payout_account_repository.go cmd/server/main.go
git add internal/domain internal/repository cmd/server/main.go
git commit -m "feat(payout): add the payout_accounts table and repository"
```

---

### Task 4: Payout usecase — encrypt on write, mask on read, decrypt only here

> **Correction (recorded after implementation):** the bank-code rule planned in this task —
> 4 or 8 characters, `^[A-Z0-9]{4}([A-Z0-9]{4})?$` — was corrected during implementation to
> **4 to 11 characters**, `^[A-Z0-9]{4,11}$`, because BCA's real BIC is `BCAIDJA` (7 characters)
> and the original rule would have rejected it. The snippet below shows the corrected rule; the
> rest of this task is unchanged.

**Files:**
- Create: `internal/usecase/payout_usecase.go`
- Create: `internal/dto/payout_dto.go`
- Test: `internal/usecase/payout_usecase_test.go`

**Interfaces:**
- Consumes: `repository.PayoutAccountRepository` (Task 3), `encryption.Cipher` (Task 1) behind the `SecretBox` interface.
- Produces: `usecase.SecretBox`, `usecase.PayoutUsecase` with `Upsert(ctx, userID uint, req dto.PayoutAccountRequest) (*entity.PayoutAccount, error)`, `Get(ctx, userID uint) (*entity.PayoutAccount, error)`, `DecryptDestination(ctx, userID uint) (usecase.PayoutDestination, error)`, `usecase.PayoutDestination{BankCode, BankName, AccountHolderName, AccountNumber string}`.

- [ ] **Step 1: Write the failing test**

Create `internal/usecase/payout_usecase_test.go` with fakes for the repository and the secret box:

```go
package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
)

type fakeSecretBox struct {
	encrypted map[string]string
	decrypted map[string]string
}

func (f *fakeSecretBox) Encrypt(plaintext string) (string, error) {
	if f.encrypted == nil {
		f.encrypted = map[string]string{}
	}
	f.encrypted[plaintext] = "enc:" + plaintext
	return "enc:" + plaintext, nil
}

func (f *fakeSecretBox) Decrypt(ciphertext string) (string, error) {
	return ciphertext[len("enc:"):], nil
}

func (f *fakeSecretBox) Fingerprint(plaintext string) string { return "fp:" + plaintext }

type fakePayoutAccountRepo struct {
	byUser map[uint]*entity.PayoutAccount
}

func (f *fakePayoutAccountRepo) Upsert(ctx context.Context, a *entity.PayoutAccount) error {
	if f.byUser == nil {
		f.byUser = map[uint]*entity.PayoutAccount{}
	}
	a.ID = 1
	f.byUser[a.UserID] = a
	return nil
}

func (f *fakePayoutAccountRepo) FindByUserID(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	a, ok := f.byUser[userID]
	if !ok {
		return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_ACCOUNT_NOT_FOUND", "payout account not found")
	}
	return a, nil
}

func (f *fakePayoutAccountRepo) FindByFingerprint(ctx context.Context, fingerprint string) (*entity.PayoutAccount, error) {
	for _, a := range f.byUser {
		if a.AccountFingerprint == fingerprint {
			return a, nil
		}
	}
	return nil, apperror.NewDomainError(apperror.ErrNotFound, "PAYOUT_ACCOUNT_NOT_FOUND", "payout account not found")
}

func TestPayoutUsecase_Upsert_EncryptsAndNeverReturnsPlaintext(t *testing.T) {
	repo := &fakePayoutAccountRepo{}
	secret := &fakeSecretBox{}
	uc := NewPayoutUsecase(repo, secret, "v1")

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode:          "BCAIDJA",
		BankName:          "Bank Central Asia",
		AccountHolderName: "Siti Rahayu",
		AccountNumber:     "1234567890",
	})
	require.NoError(t, err)

	assert.Equal(t, "enc:1234567890", account.AccountNumberEncrypted)
	assert.Equal(t, "enc:Siti Rahayu", account.AccountHolderNameEncrypted)
	assert.Equal(t, "7890", account.AccountLast4)
	assert.Equal(t, "fp:1234567890", account.AccountFingerprint)
	assert.Equal(t, entity.PayoutAccountStatusUnverified, account.Status)
}

func TestPayoutUsecase_Upsert_RejectsAnAccountAlreadyUsedByAnotherMitra(t *testing.T) {
	repo := &fakePayoutAccountRepo{}
	uc := NewPayoutUsecase(repo, &fakeSecretBox{}, "v1")

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti", AccountNumber: "1234567890",
	})
	require.NoError(t, err)

	_, err = uc.Upsert(context.Background(), 72, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Rudi", AccountNumber: "1234567890",
	})
	require.Error(t, err)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "PAYOUT_ACCOUNT_DUPLICATE", domainErr.Code)
}

func TestPayoutUsecase_Upsert_RejectsInvalidBankCodeAndAccountNumber(t *testing.T) {
	uc := NewPayoutUsecase(&fakePayoutAccountRepo{}, &fakeSecretBox{}, "v1")

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BC", BankName: "Bank", AccountHolderName: "Siti", AccountNumber: "1234567890",
	})
	assert.Error(t, err)

	_, err = uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti", AccountNumber: "12-34",
	})
	assert.Error(t, err)
}

func TestPayoutUsecase_Upsert_FlagsNameMismatchWithoutFailing(t *testing.T) {
	uc := NewPayoutUsecase(&fakePayoutAccountRepo{}, &fakeSecretBox{}, "v1")

	account, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "PT Contoh", AccountNumber: "1234567890",
	})
	require.NoError(t, err)
	assert.True(t, account.NameMismatch)
}

func TestPayoutUsecase_DecryptDestination_ReturnsPlaintextOnlyHere(t *testing.T) {
	repo := &fakePayoutAccountRepo{}
	uc := NewPayoutUsecase(repo, &fakeSecretBox{}, "v1")

	_, err := uc.Upsert(context.Background(), 71, dto.PayoutAccountRequest{
		BankCode: "BCAIDJA", BankName: "Bank", AccountHolderName: "Siti Rahayu", AccountNumber: "1234567890",
	})
	require.NoError(t, err)

	destination, err := uc.DecryptDestination(context.Background(), 71)
	require.NoError(t, err)
	assert.Equal(t, "1234567890", destination.AccountNumber)
	assert.Equal(t, "Siti Rahayu", destination.AccountHolderName)
	assert.Equal(t, "BCAIDJA", destination.BankCode)
}
```

Add a `fakePayoutAccountRepo` in the same file implementing the three repository methods with an in-memory map (code above).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/usecase/ -run TestPayoutUsecase`
Expected: FAIL — `NewPayoutUsecase` undefined.

- [ ] **Step 3: Write the implementation**

`internal/dto/payout_dto.go`:

```go
package dto

import (
	"errors"
	"regexp"
	"strings"
)

var (
	bankCodePattern    = regexp.MustCompile(`^[A-Z0-9]{4,11}$`)
	accountNumberPattern = regexp.MustCompile(`^[0-9]{6,32}$`)
)

type PayoutAccountRequest struct {
	BankCode          string `json:"bank_code" validate:"required"`
	BankName          string `json:"bank_name" validate:"required"`
	AccountHolderName string `json:"account_holder_name" validate:"required"`
	AccountNumber     string `json:"account_number" validate:"required"`
}

// Validate enforces the shapes DOKU expects: a SWIFT/BIC bank code and a
// numeric account number.
func (r PayoutAccountRequest) Validate() error {
	if !bankCodePattern.MatchString(r.BankCode) {
		return errors.New("bank_code must be 4 to 11 uppercase letters or digits")
	}
	if !accountNumberPattern.MatchString(r.AccountNumber) {
		return errors.New("account_number must be 6 to 32 digits")
	}
	if len(strings.TrimSpace(r.AccountHolderName)) < 2 {
		return errors.New("account_holder_name is required")
	}
	return nil
}

type PayoutAccountResponse struct {
	BankCode     string `json:"bank_code"`
	BankName     string `json:"bank_name"`
	AccountLast4 string `json:"account_last4"`
	Status       string `json:"status"`
	NameMismatch bool   `json:"name_mismatch"`
	UpdatedAt    string `json:"updated_at"`
}
```

`internal/usecase/payout_usecase.go`:

```go
package usecase

import (
	"context"
	"strings"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

// SecretBox is the encryption boundary the payout usecase depends on.
type SecretBox interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
	Fingerprint(plaintext string) string
}

// PayoutDestination is the decrypted form, used only when paying out.
type PayoutDestination struct {
	BankCode          string
	BankName          string
	AccountHolderName string
	AccountNumber     string
}

type PayoutUsecase struct {
	repo           repository.PayoutAccountRepository
	secret         SecretBox
	encryptionKeyID string
}

func NewPayoutUsecase(repo repository.PayoutAccountRepository, secret SecretBox, encryptionKeyID string) *PayoutUsecase {
	if encryptionKeyID == "" {
		encryptionKeyID = "v1"
	}
	return &PayoutUsecase{repo: repo, secret: secret, encryptionKeyID: encryptionKeyID}
}

func (uc *PayoutUsecase) Upsert(ctx context.Context, userID uint, req dto.PayoutAccountRequest) (*entity.PayoutAccount, error) {
	if err := req.Validate(); err != nil {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "INVALID_PAYOUT_ACCOUNT", err.Error())
	}

	fingerprint := uc.secret.Fingerprint(req.AccountNumber)
	// Two Mitras must not share one bank account: the payout would go to
	// one of them. The fingerprint detects this without decrypting anything.
	existing, err := uc.repo.FindByFingerprint(ctx, fingerprint)
	if err == nil && existing != nil && existing.UserID != userID {
		return nil, apperror.NewDomainError(apperror.ErrValidation, "PAYOUT_ACCOUNT_DUPLICATE", "this bank account is already used by another mitra")
	}
	if err != nil && !isNotFound(err) {
		return nil, err
	}

	encryptedNumber, err := uc.secret.Encrypt(req.AccountNumber)
	if err != nil {
		return nil, err
	}
	encryptedHolder, err := uc.secret.Encrypt(req.AccountHolderName)
	if err != nil {
		return nil, err
	}

	account := &entity.PayoutAccount{
		UserID:                    userID,
		BankCode:                  strings.ToUpper(strings.TrimSpace(req.BankCode)),
		BankName:                  strings.TrimSpace(req.BankName),
		AccountHolderNameEncrypted: encryptedHolder,
		AccountNumberEncrypted:    encryptedNumber,
		AccountLast4:              lastFour(req.AccountNumber),
		AccountFingerprint:        fingerprint,
		EncryptionKeyID:           uc.encryptionKeyID,
		Status:                    entity.PayoutAccountStatusUnverified,
	}
	if err := uc.repo.Upsert(ctx, account); err != nil {
		return nil, err
	}
	return account, nil
}

// isNotFound reports whether the error is the repository's "no such payout
// account" error, which is the expected result of the duplicate lookup.
func isNotFound(err error) bool {
	var domainErr *apperror.DomainError
	if errors.As(err, &domainErr) {
		return errors.Is(domainErr.Unwrap(), apperror.ErrNotFound)
	}
	return false
}

// lastFour returns the last four characters of an account number for masked
// display. The full value is never stored in plaintext.
func lastFour(accountNumber string) string {
	if len(accountNumber) <= 4 {
		return accountNumber
	}
	return accountNumber[len(accountNumber)-4:]
}
```

`Get` returns the stored account (still encrypted) so the handler can mask it; `DecryptDestination` decrypts both fields and returns them, and is the only place in the codebase that does:

```go
func (uc *PayoutUsecase) Get(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	return uc.repo.FindByUserID(ctx, userID)
}

// DecryptDestination returns the plain destination for a payout. Only the
// future transfer path may call it; it never reaches a handler.
func (uc *PayoutUsecase) DecryptDestination(ctx context.Context, userID uint) (PayoutDestination, error) {
	account, err := uc.repo.FindByUserID(ctx, userID)
	if err != nil {
		return PayoutDestination{}, err
	}
	number, err := uc.secret.Decrypt(account.AccountNumberEncrypted)
	if err != nil {
		return PayoutDestination{}, err
	}
	holder, err := uc.secret.Decrypt(account.AccountHolderNameEncrypted)
	if err != nil {
		return PayoutDestination{}, err
	}
	return PayoutDestination{
		BankCode:          account.BankCode,
		BankName:          account.BankName,
		AccountHolderName: holder,
		AccountNumber:     number,
	}, nil
}
```

The name-mismatch flag is set by comparing the normalised holder name with the registered account name. Because the production entity (PT/CV) is unresolved, a mismatch is **flagged, not blocked**: the row is stored with `name_mismatch = true` and `status = unverified`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/usecase/ -run TestPayoutUsecase -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/usecase/payout_usecase.go internal/dto/payout_dto.go internal/usecase/payout_usecase_test.go
git add internal/usecase internal/dto
git commit -m "feat(payout): encrypt the Mitra bank destination on write"
```

---

### Task 5: HTTP surface for the Mitra's payout destination

**Files:**
- Create: `internal/handler/payout_handler.go`
- Modify: `cmd/server/main.go` (handler + routes)
- Test: `internal/handler/payout_handler_test.go`

**Interfaces:**
- Consumes: `usecase.PayoutUsecase` (Task 4), `usecase.ProfileUsecase` for the role check.
- Produces: `PUT /api/v1/payout-account`, `GET /api/v1/payout-account`.

- [ ] **Step 1: Write the failing test**

Create `internal/handler/payout_handler_test.go`:

```go
package handler

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

func TestPayoutHandler_Upsert_ReturnsMaskedAccount(t *testing.T) {
	uc := &fakePayoutUsecase{account: &entity.PayoutAccount{
		UserID: 71, BankCode: "BCAIDJA", BankName: "Bank Central Asia",
		AccountNumberEncrypted: "enc-secret", AccountHolderNameEncrypted: "enc-holder",
		AccountLast4: "7890", Status: entity.PayoutAccountStatusUnverified,
	}}
	h := NewPayoutHandler(uc, &fakeProfileUsecase{role: "provider"})

	app := fiber.New()
	app.Put("/api/v1/payout-account", h.Upsert)

	req := httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(
		`{"bank_code":"BCAIDJA","bank_name":"Bank Central Asia","account_holder_name":"Siti Rahayu","account_number":"1234567890"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)

	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, body, "7890")
	assert.NotContains(t, body, "1234567890")
	assert.NotContains(t, body, "enc-secret")
	assert.NotContains(t, body, "Siti Rahayu")
}

func TestPayoutHandler_Upsert_RejectsNonProvider(t *testing.T) {
	uc := &fakePayoutUsecase{}
	h := NewPayoutHandler(uc, &fakeProfileUsecase{role: "customer"})

	app := fiber.New()
	app.Put("/api/v1/payout-account", h.Upsert)

	req := httptest.NewRequest("PUT", "/api/v1/payout-account", strings.NewReader(
		`{"bank_code":"BCAIDJA","bank_name":"Bank Central Asia","account_holder_name":"Siti Rahayu","account_number":"1234567890"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode)
	assert.False(t, uc.upsertCalled, "a customer must not be able to create a payout destination")
}
```

The fakes used by both tests:

```go
type fakePayoutUsecase struct {
	account      *entity.PayoutAccount
	upsertCalled bool
}

func (f *fakePayoutUsecase) Upsert(ctx context.Context, userID uint, req dto.PayoutAccountRequest) (*entity.PayoutAccount, error) {
	f.upsertCalled = true
	return f.account, nil
}

func (f *fakePayoutUsecase) Get(ctx context.Context, userID uint) (*entity.PayoutAccount, error) {
	return f.account, nil
}

func (f *fakePayoutUsecase) DecryptDestination(ctx context.Context, userID uint) (usecase.PayoutDestination, error) {
	return usecase.PayoutDestination{}, nil
}

type fakeProfileUsecase struct {
	role string
}

func (f *fakeProfileUsecase) GetProfile(ctx context.Context, userID uint) (*entity.User, error) {
	return &entity.User{ID: userID, Role: f.role}, nil
}
```

If `NewPayoutHandler` takes the concrete `*usecase.PayoutUsecase`, use the interface
`usecase.PayoutAccountService` (Upsert, Get, DecryptDestination) instead so the
handler can be tested with the fakes above.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/handler/ -run TestPayoutHandler`
Expected: FAIL — `NewPayoutHandler` undefined.

- [ ] **Step 3: Write the implementation**

`internal/handler/payout_handler.go`:

```go
package handler

import (
	"github.com/gofiber/fiber/v3"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

type PayoutHandler struct {
	payoutUsecase *usecase.PayoutUsecase
	profileUsecase *usecase.ProfileUsecase
}

func NewPayoutHandler(payoutUsecase *usecase.PayoutUsecase, profileUsecase *usecase.ProfileUsecase) *PayoutHandler {
	return &PayoutHandler{payoutUsecase: payoutUsecase, profileUsecase: profileUsecase}
}

// requireProvider rejects anyone who is not a Mitra: only a provider gets
// paid, so only a provider may set a payout destination.
func (h *PayoutHandler) requireProvider(c fiber.Ctx) (uint, bool) {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
		return 0, false
	}
	profile, err := h.profileUsecase.GetProfile(c.Context(), userID)
	if err != nil || profile == nil || profile.Role != "provider" {
		apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusForbidden, "FORBIDDEN", "only a mitra can set a payout account"))
		return 0, false
	}
	return userID, true
}

func (h *PayoutHandler) Upsert(c fiber.Ctx) error {
	userID, ok := h.requireProvider(c)
	if !ok {
		return nil
	}

	var req dto.PayoutAccountRequest
	if err := c.Bind().Body(&req); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body"))
	}
	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	account, err := h.payoutUsecase.Upsert(c.Context(), userID, req)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return response.Success(c, fiber.StatusOK, toPayoutAccountResponse(account))
}

func (h *PayoutHandler) Get(c fiber.Ctx) error {
	userID, ok := h.requireProvider(c)
	if !ok {
		return nil
	}
	account, err := h.payoutUsecase.Get(c.Context(), userID)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, toPayoutAccountResponse(account))
}

// toPayoutAccountResponse is the only shape a payout account leaves the API
// in: never the account number, never the holder name.
func toPayoutAccountResponse(account *entity.PayoutAccount) dto.PayoutAccountResponse {
	return dto.PayoutAccountResponse{
		BankCode:     account.BankCode,
		BankName:     account.BankName,
		AccountLast4: account.AccountLast4,
		Status:       account.Status,
		NameMismatch: account.NameMismatch,
		UpdatedAt:    account.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
```

In `cmd/server/main.go`:

```go
	payoutUsecase := usecase.NewPayoutUsecase(payoutRepo, payoutCipher, cfg.PayoutEncryptionKeyID)
	payoutHandler := handler.NewPayoutHandler(payoutUsecase, profileUsecase)
```

```go
	protected.Put("/payout-account", payoutHandler.Upsert)
	protected.Get("/payout-account", payoutHandler.Get)
```

Build the cipher in `main.go` right after the config check:

```go
	payoutCipher, err := encryption.NewCipher(cfg.PayoutEncryptionKey, cfg.PayoutEncryptionKeyID)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to initialise the payout encryption cipher")
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/handler/ -run TestPayoutHandler -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/handler/payout_handler.go internal/handler/payout_handler_test.go cmd/server/main.go
git add internal/handler cmd/server/main.go
git commit -m "feat(payout): expose the masked payout destination to the Mitra"
```

---

### Task 6: Mitra profile — bank destination form

**Files:**
- Create: `src/services/payoutAccount.service.js`
- Create: `src/pages/profile/components/PayoutAccountSection.jsx`
- Create: `src/pages/profile/hooks/usePayoutAccount.js`
- Modify: `src/pages/profile/index.jsx`
- Test: `src/__tests__/pages/PayoutAccountSection.test.jsx`

**Interfaces:**
- Consumes: `PUT/GET /api/v1/payout-account` (Task 5).
- Produces: `payoutAccountService.get()` and `payoutAccountService.save(payload)`.

- [ ] **Step 1: Write the failing test**

Create `src/__tests__/pages/PayoutAccountSection.test.jsx`:

```jsx
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const mockGet = vi.fn()
const mockSave = vi.fn()

vi.mock('../../services/payoutAccount.service', () => ({
  payoutAccountService: {
    get: (...args) => mockGet(...args),
    save: (...args) => mockSave(...args),
  },
}))

import PayoutAccountSection from '../../pages/profile/components/PayoutAccountSection'

describe('PayoutAccountSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockResolvedValue({
      bank_code: 'BCAIDJA',
      bank_name: 'Bank Central Asia',
      account_last4: '7890',
      status: 'unverified',
      name_mismatch: false,
    })
  })

  it('shows only the masked account of a saved destination', async () => {
    render(<PayoutAccountSection />)

    await waitFor(() => expect(screen.getByText(/7890/)).toBeInTheDocument())
    expect(screen.getByText(/Belum diverifikasi/)).toBeInTheDocument()
    expect(screen.queryByText(/1234567890/)).not.toBeInTheDocument()
  })

  it('saves the destination and shows the masked result', async () => {
    mockSave.mockResolvedValue({ bank_code: 'BCAIDJA', bank_name: 'Bank Central Asia', account_last4: '7890', status: 'unverified' })
    render(<PayoutAccountSection />)
    await waitFor(() => expect(mockGet).toHaveBeenCalled())

    const user = userEvent.setup()
    await user.type(screen.getByLabelText(/Kode bank/i), 'BCAIDJA')
    await user.type(screen.getByLabelText(/Nama bank/i), 'Bank Central Asia')
    await user.type(screen.getByLabelText(/Nama pemilik rekening/i), 'Siti Rahayu')
    await user.type(screen.getByLabelText(/Nomor rekening/i), '1234567890')
    await user.click(screen.getByRole('button', { name: /Simpan/i }))

    await waitFor(() => expect(mockSave).toHaveBeenCalledWith({
      bank_code: 'BCAIDJA',
      bank_name: 'Bank Central Asia',
      account_holder_name: 'Siti Rahayu',
      account_number: '1234567890',
    }))
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npx vitest run src/__tests__/pages/PayoutAccountSection.test.jsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Write the implementation**

`src/services/payoutAccount.service.js`:

```js
import api from './api'

export const payoutAccountService = {
  async get() {
    const response = await api.get('/api/v1/payout-account')
    return response.data.data
  },

  async save(payload) {
    const response = await api.put('/api/v1/payout-account', payload)
    return response.data.data
  },
}
```

`src/pages/profile/hooks/usePayoutAccount.js`:

```js
import { useCallback, useEffect, useState } from 'react'
import { payoutAccountService } from '../../../services/payoutAccount.service'

export function usePayoutAccount() {
  const [account, setAccount] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      const data = await payoutAccountService.get()
      setAccount(data || null)
    } catch (err) {
      // A Mitra without a destination yet gets a 404; that is not an error.
      if (err?.response?.status !== 404) {
        setError(err?.response?.data?.message || 'Gagal memuat data rekening.')
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  const save = useCallback(async (payload) => {
    setSaving(true)
    setError('')
    try {
      const saved = await payoutAccountService.save(payload)
      setAccount(saved)
      return saved
    } catch (err) {
      setError(err?.response?.data?.message || 'Gagal menyimpan rekening.')
      return null
    } finally {
      setSaving(false)
    }
  }, [])

  return { account, loading, saving, error, save, reload: load }
}
```

`src/pages/profile/components/PayoutAccountSection.jsx` — a card with the form and the masked state, using the existing section styling (`bg-white rounded-2xl border border-warm-border p-5`), labelled inputs (`htmlFor` + `id`) so the test can find them, and copy that says the destination is encrypted:

```jsx
import { useEffect, useState } from 'react'
import { Lock, Wallet } from 'lucide-react'
import { usePayoutAccount } from '../hooks/usePayoutAccount'

const EMPTY = { bank_code: '', bank_name: '', account_holder_name: '', account_number: '' }

export default function PayoutAccountSection() {
  const { account, loading, saving, error, save } = usePayoutAccount()
  const [form, setForm] = useState(EMPTY)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (account) {
      setForm((prev) => ({
        ...prev,
        bank_code: account.bank_code || '',
        bank_name: account.bank_name || '',
      }))
    }
  }, [account])

  function update(field, value) {
    setForm((prev) => ({ ...prev, [field]: value }))
  }

  async function handleSubmit(e) {
    e.preventDefault()
    const result = await save(form)
    if (result) {
      // The number is never read back from the server: clear it locally.
      setForm((prev) => ({ ...prev, account_number: '' }))
      setSaved(true)
    }
  }

  return (
    <section className="bg-white rounded-2xl border border-warm-border p-5">
      <h2 className="text-sm font-bold text-gray-900 mb-1 flex items-center gap-2">
        <Wallet size={16} className="text-primary" />
        Rekening payouts
      </h2>
      <p className="text-xs text-gray-500 mb-4">
        Rekening ini dipakai DOKU untuk mengirim bayaran Anda. Nomor rekening disimpan terenkripsi.
      </p>

      {account && (
        <div className="bg-soft-surface border border-warm-border rounded-xl px-4 py-3 mb-4 text-sm flex items-center justify-between gap-3">
          <div>
            <p className="font-semibold text-gray-900">{account.bank_name} · {account.bank_code}</p>
            <p className="text-xs text-gray-500 mt-0.5">Nomor rekening tersimpan berakhiran {account.account_last4}</p>
          </div>
          <span className="text-[10px] font-bold text-amber-700 bg-amber-50 rounded-full px-2 py-1">
            Belum diverifikasi
          </span>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label htmlFor="payout-bank-code" className="text-xs font-semibold text-gray-600 mb-1.5 block">
            Kode bank
          </label>
          <input
            id="payout-bank-code"
            value={form.bank_code}
            onChange={(e) => update('bank_code', e.target.value)}
            placeholder="BCAIDJA"
            maxLength={8}
            className="form-input w-full bg-white border border-warm-border rounded-xl px-4 py-3 text-sm text-gray-800"
          />
        </div>
        <div>
          <label htmlFor="payout-bank-name" className="text-xs font-semibold text-gray-600 mb-1.5 block">
            Nama bank
          </label>
          <input
            id="payout-bank-name"
            value={form.bank_name}
            onChange={(e) => update('bank_name', e.target.value)}
            placeholder="Bank Central Asia"
            maxLength={100}
            className="form-input w-full bg-white border border-warm-border rounded-xl px-4 py-3 text-sm text-gray-800"
          />
        </div>
        <div>
          <label htmlFor="payout-holder-name" className="text-xs font-semibold text-gray-600 mb-1.5 block">
            Nama pemilik rekening
          </label>
          <input
            id="payout-holder-name"
            value={form.account_holder_name}
            onChange={(e) => update('account_holder_name', e.target.value)}
            placeholder="Nama sesuai rekening bank"
            maxLength={100}
            className="form-input w-full bg-white border border-warm-border rounded-xl px-4 py-3 text-sm text-gray-800"
          />
        </div>
        <div>
          <label htmlFor="payout-account-number" className="text-xs font-semibold text-gray-600 mb-1.5 block">
            Nomor rekening
          </label>
          <input
            id="payout-account-number"
            value={form.account_number}
            onChange={(e) => update('account_number', e.target.value)}
            placeholder="1234567890"
            inputMode="numeric"
            maxLength={32}
            className="form-input w-full bg-white border border-warm-border rounded-xl px-4 py-3 text-sm text-gray-800"
          />
        </div>

        <button
          type="submit"
          disabled={saving || loading}
          className="btn-press w-full bg-primary hover:bg-primary-dark text-white text-sm font-bold py-3 rounded-xl transition-colors disabled:opacity-50"
        >
          {saving ? 'Menyimpan...' : 'Simpan rekening'}
        </button>
      </form>

      {error && <p className="text-xs text-red-600 mt-3">{error}</p>}
      {saved && <p className="text-xs text-green-700 mt-3">Rekening tersimpan.</p>}

      <p className="text-[11px] text-gray-400 mt-4 flex items-start gap-1.5">
        <Lock size={12} className="mt-0.5 shrink-0" />
        Nomor rekening dienkripsi dan tidak pernah ditampilkan kembali.
      </p>
    </section>
  )
}
```

Render it in `src/pages/profile/index.jsx` only for a provider:

```jsx
{user?.role === 'provider' && <PayoutAccountSection />}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `npx vitest run src/__tests__/pages/PayoutAccountSection.test.jsx`
Expected: PASS.

Then: `npm run test:run`, `npm run lint`, `npm run build`.

- [ ] **Step 5: Commit**

```bash
git add src/services/payoutAccount.service.js src/pages/profile src/__tests__/pages/PayoutAccountSection.test.jsx
git commit -m "feat(payout): let a Mitra set an encrypted payout destination"
```

---

### Task 7: Contract and documentation

**Files:**
- Modify: `serabutin-technical-documentation/api-contracts/openapi.yaml`
- Create: `serabutin-technical-documentation/spec/017.payout-bank-details.md`
- Modify: `serabutin-technical-documentation/spec/016.payments-prepay-doku.md`
- Modify: `serabutin-technical-documentation/research/2026-10-18-doku-payment-gateway-assessment.md` (rev. 5)
- Modify: `serabutin-technical-documentation/spec/handoff/2026-10-24-prepay-doku-checkout.md`

- [ ] **Step 1: Add the OpenAPI paths**

Add to `api-contracts/openapi.yaml`:

```yaml
  /api/v1/payout-account:
    get:
      tags: [Profile]
      summary: Read the caller's payout destination (masked)
      description: |
        The Mitra's bank destination for DOKU payouts. The account number and
        the holder name are stored encrypted and are never returned; only the
        bank, the last four digits and the validation status are.
      operationId: getPayoutAccount
      security:
        - bearerAuth: []
      responses:
        '200':
          description: The stored destination, masked.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/PayoutAccountResponse'
        '401':
          description: Missing or invalid bearer token.
        '403':
          description: The caller is not a mitra.
        '404':
          description: No payout destination stored yet.
    put:
      tags: [Profile]
      summary: Create or replace the caller's payout destination
      description: |
        Stores the Mitra's bank destination. The account number and the holder
        name are encrypted before they are written. The destination starts as
        `unverified`: DOKU account validation is not wired yet.
      operationId: savePayoutAccount
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/PayoutAccountRequest'
      responses:
        '200':
          description: The stored destination, masked.
        '401':
          description: Missing or invalid bearer token.
        '403':
          description: The caller is not a mitra.
        '422':
          description: Invalid bank code or account number.
```

and the schemas:

```yaml
    PayoutAccountRequest:
      type: object
      required: [bank_code, bank_name, account_holder_name, account_number]
      properties:
        bank_code:
          type: string
          description: SWIFT/BIC code of the bank.
          pattern: '^[A-Z0-9]{4}([A-Z0-9]{4})?$'
          example: BCAIDJA
        bank_name:
          type: string
          maxLength: 100
          example: Bank Central Asia
        account_holder_name:
          type: string
          maxLength: 100
          example: Siti Rahayu
        account_number:
          type: string
          pattern: '^[0-9]{6,32}$'
          example: '1234567890'

    PayoutAccountResponse:
      type: object
      description: The stored payout destination. Never contains the account number or the holder name.
      required: [bank_code, bank_name, account_last4, status]
      properties:
        bank_code:
          type: string
          example: BCAIDJA
        bank_name:
          type: string
          example: Bank Central Asia
        account_last4:
          type: string
          example: '7890'
        status:
          type: string
          enum: [unverified, verified, failed]
        name_mismatch:
          type: boolean
          description: The holder name differs from the registered account name.
        updated_at:
          type: string
          format: date-time
```

Validate the file still parses:

```bash
cd serabutin-technical-documentation
python3 -c "import yaml;d=yaml.safe_load(open('api-contracts/openapi.yaml'));print(len(d['paths']))"
```

- [ ] **Step 2: Write spec 017**

Create `serabutin-technical-documentation/spec/017.payout-bank-details.md` with these sections and these facts (no other content is needed):

- **1. Prompt** — the founder's request: every Mitra needs a bank destination for DOKU to pay them, encrypted, with the key in the environment.
- **2. Goal** — the platform can hold a verified payout destination per Mitra so DOKU's `transfer-payment` with `type: BANK_ACCOUNT` can pay the Mitra directly from the hub sub-account, without a DOKU sub-account per Mitra.
- **3. Why** — the founder's decision that Mitras do not get DOKU sub-accounts; the platform therefore owns collecting, encrypting and validating the bank destination. Reference the design doc `serabutin-id-backend/docs/superpowers/specs/2026-10-28-payout-bank-details-design.md` and spec 016.
- **4. Codebase Analysis** — the users table stores no bank details today; the profile usecase returns `*entity.User` with `Role`; `payout_marks` is the unrelated legacy admin payout ledger; the payment tables and webhook flow already exist from spec 016.
- **5. Logic Changes** — the table, which fields are encrypted, the key handling (required, fail-closed, `PAYOUT_ENCRYPTION_KEY_ID` for rotation), the API surface, the validation rules, and the decryption boundary.
- **6. Code Changes** — the per-file table from the implementation plan (entity, model, repository, usecase, handler, dto, config, main, frontend section).
- **7. Pseudocode** — upsert (validate → fingerprint → duplicate check → encrypt → store), read (masked), decrypt destination (payout path only).
- **8. Test Simulation** — the five Review Focus cases: missing key, duplicate account, masked read, customer calling the endpoint, name mismatch.
- **9. Manual Testing Plan** — generate a key, start the backend, sign in as a Mitra, save a destination, confirm the response shows only the last four digits, confirm the row in the database is ciphertext, confirm the customer role gets 403.
- **10. Open items** — DOKU's Bank Account Validation (the `verified` transition), the `transfer-inquiry`/`transfer-payment` release path, e-wallet payouts (not available from a sub-account), settlement timing (QRIS T+1) and the float it creates, the unresolved production entity behind the name-mismatch rule, and key rotation.

- [ ] **Step 3: Update the research and spec 016**

In `research/2026-10-18-doku-payment-gateway-assessment.md`, add rev. 5 noting the founder's decision that **Mitra do not get DOKU sub-accounts**: the release is `transfer-payment` with `type: BANK_ACCOUNT` from the hub sub-account straight to the Mitra's bank account, which requires the platform to collect and protect each Mitra's bank destination. Note the consequences: the payout cannot leave the hub before settlement (QRIS T+1), the platform carries the float, per-Mitra KYC at DOKU is skipped (so we own bank verification), and e-wallet payouts to Mitras are unavailable from a sub-account.

In `spec/016.payments-prepay-doku.md` §10.3.1, record that the release target is the bank payout and that the Mitra's destination is managed by spec 017.

In `spec/handoff/2026-10-24-prepay-doku-checkout.md`, add the payout-destination work to "What I did" and the outstanding refund/settlement items to "What's pending".

- [ ] **Step 4: Commit**

```bash
cd serabutin-technical-documentation
git add api-contracts/openapi.yaml spec/017.payout-bank-details.md spec/016.payments-prepay-doku.md research/ spec/handoff/
git commit -m "docs(payout): encrypted Mitra payout destination — spec, contract, research rev 5"
```

---

## Execution Notes

- Run the backend suite (`go test ./...`) after Tasks 1–5; the live tests need Postgres and Redis up (`docker compose up -d postgres redis`).
- The frontend suite is `npm run test:run`; lint must stay at 0 errors.
- Keep the two repositories' commits separate: the backend owns Tasks 1–5, the frontend owns Task 6, the documentation repo owns Task 7.

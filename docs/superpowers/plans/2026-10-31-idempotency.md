# Idempotency for every money movement — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every money-moving call to DOKU safe to retry: a repeated collection creates no second invoice, and a repeated withdrawal pays the Mitra once.

**Architecture:** Two parts sharing one convention. Every attempt to move money is a persisted row (a `payments` row for collection, a new `payouts` row for withdrawal) that carries a **derived** idempotency key and a request fingerprint, written **before** the provider call. The key is sent to DOKU as its own idempotency key (`Request-Id` for Checkout, `partnerReferenceNo` for the Sub-Account transfer API), so a retry is deduplicated by the provider. A provider `409` carrying the original result is read as a **replay**, not a failure.

**Tech Stack:** Go 1.25, GoFiber v3, GORM/Postgres, testify, testcontainers-go; React 19 + Vite + Vitest on the frontend.

**Spec:** `docs/superpowers/specs/2026-10-31-idempotency-design.md`

## Global Constraints

- **No idempotency key is ever generated at call time.** Every key is derived from a persisted row, so a retry reuses it by construction.
- Collection key: `SRB-<bookingID>-<paymentID>`. Withdrawal key: `PO-<bookingID>`. The withdrawal's secondary provider key `X-EXTERNAL-ID` is the numeric `payouts.id`.
- A key is unique per row. A key reused with a different request is rejected with `IDEMPOTENCY_KEY_REUSED` (never silently reused).
- The request fingerprint is `SHA-256(canonical-request)`, hex, truncated to 32 characters. It is a reuse detector, not a secret: the key sits in the same row. Secret material (the payout beneficiary account number) uses the encryption-keyed digest instead.
- A provider `409` whose body carries the original result is **success**; the attempt is marked `replayed`. A `409` without it is an error.
- The beneficiary account number is encrypted at rest with the existing `PAYOUT_ENCRYPTION_KEY`, exactly like the payout destination.
- No idempotency key, fingerprint or reference number may contain an account number, a secret, or any other secret material.
- The `X-EXTERNAL-ID` header must be a **numeric** string (DOKU requirement).
- Go files `gofmt` clean; `go test ./...` green. Frontend: `npm run test:run`, `npm run lint` (0 errors), `npm run build`.
- TDD per task: the failing test first, then the implementation, then the commit.

---

## Part A — Collection idempotency

### Task 1 (Part A): Key derivation and request fingerprint

**Files:**
- Create: `internal/infrastructure/idempotency/keys.go`
- Test: `internal/infrastructure/idempotency/keys_test.go`

**Interfaces:**
- Produces: `idempotency.CollectionKey(bookingID, paymentID uint) string`, `idempotency.PayoutKey(bookingID uint) string`, `idempotency.Fingerprint(key, canonicalRequest string) string`.

- [ ] **Step 1: Write the failing test**

`internal/infrastructure/idempotency/keys_test.go`:

```go
package idempotency

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollectionKey_IsStableAndUniquePerPayment(t *testing.T) {
	assert.Equal(t, "SRB-42-7", CollectionKey(42, 7))
	assert.Equal(t, "SRB-42-7", CollectionKey(42, 7))
	assert.NotEqual(t, CollectionKey(42, 7), CollectionKey(42, 8))
	assert.NotEqual(t, CollectionKey(42, 7), CollectionKey(43, 7))
}

func TestPayoutKey_IsStablePerBooking(t *testing.T) {
	assert.Equal(t, "PO-42", PayoutKey(42))
	assert.NotEqual(t, PayoutKey(42), PayoutKey(43))
}

func TestFingerprint_IsStableForTheSameRequestAndDiffersOtherwise(t *testing.T) {
	first := Fingerprint("SRB-42-7", `{"amount":155000,"channel":"qris"}`)
	assert.Equal(t, first, Fingerprint("SRB-42-7", `{"amount":155000,"channel":"qris"}`))
	assert.NotEqual(t, first, Fingerprint("SRB-42-7", `{"amount":155001,"channel":"qris"}`))
	assert.Len(t, first, 32)
	assert.NotContains(t, first, "155000")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/infrastructure/idempotency/...`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write the implementation**

`internal/infrastructure/idempotency/keys.go`:

```go
package idempotency

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// CollectionKey is the idempotency key of a checkout attempt. It is derived
// from the persisted payment row, so a retry of that attempt reuses it and a
// genuinely new attempt (a new payment row) gets a new key.
func CollectionKey(bookingID, paymentID uint) string {
	return fmt.Sprintf("SRB-%d-%d", bookingID, paymentID)
}

// PayoutKey is the idempotency key of the withdrawal of one booking. At most
// one payout row exists per booking, so the key is unique by construction.
func PayoutKey(bookingID uint) string {
	return fmt.Sprintf("PO-%d", bookingID)
}

// Fingerprint identifies the request a key was used for: a plain SHA-256
// hex digest of the canonical request, truncated to 32 characters. It is a
// reuse detector, not a confidentiality control, because the key is stored
// beside it. Never pass secret material here; use the encryption-keyed digest.
func Fingerprint(canonicalRequest string) string {
	sum := sha256.Sum256([]byte(canonicalRequest))
	return hex.EncodeToString(sum[:])[:32]
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/infrastructure/idempotency/ -count=1 -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/infrastructure/idempotency/
git add internal/infrastructure/idempotency/
git commit -m "feat(payments): derive idempotency keys and request fingerprints"
```

---

### Task 2 (Part A): Persist the key and fingerprint on the payment

**Files:**
- Modify: `internal/domain/entity/payment.go`
- Modify: `internal/repository/model/payment_model.go`
- Modify: `internal/repository/gorm/payment_repository.go`
- Modify: `internal/domain/repository/payout_account_repository.go` (no change — the interface file for payments is `internal/domain/repository/payment_repository.go`)
- Test: `internal/repository/gorm/payment_live_test.go`

**Interfaces:**
- Produces on `entity.Payment`: `IdempotencyKey string`, `RequestFingerprint string`, `Replayed bool`.
- New repository method: `FindByIdempotencyKey(ctx, key string) (*entity.Payment, error)` — added to the `repository.PaymentRepository` **interface** (not only the concrete implementation), with the two existing fakes in `internal/usecase/payment_usecase_test.go` and `internal/handler/payment_handler_test.go` updated to satisfy it.
- New repository methods for Task 3: `UpdateIdempotency(ctx, paymentID uint, key, fingerprint string) error` and `MarkReplayed(ctx, paymentID uint, replayed bool) error`.

- [ ] **Step 1: Write the failing test**

In a new `internal/repository/gorm/payment_live_test.go` (reuse the package's shared live Postgres helper `setupBountyLiveDB(t)`, adding `&model.PaymentModel{}` to its `AutoMigrate` list if it is not there yet):

```go
func TestLivePayment_FindByIdempotencyKey(t *testing.T) {
	db := setupBountyLiveDB(t)
	ctx := context.Background()
	repo := NewPaymentRepository(db)

	payment := &entity.Payment{
		BookingID:         42,
		InvoiceNumber:     "SRB-42-7-1",
		Amount:            155000,
		Currency:          "IDR",
		Channel:           "qris",
		Status:            entity.PaymentStatusPending,
		Provider:          "doku",
		IdempotencyKey:    "SRB-42-7",
		RequestFingerprint: "fp-1",
	}
	require.NoError(t, repo.Create(ctx, payment))

	found, err := repo.FindByIdempotencyKey(ctx, "SRB-42-7")
	require.NoError(t, err)
	assert.Equal(t, uint(42), found.BookingID)
	assert.Equal(t, "fp-1", found.RequestFingerprint)

	_, err = repo.FindByIdempotencyKey(ctx, "SRB-99-9")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/repository/gorm/ -run TestLivePayment`
Expected: FAIL — the entity fields and the repository method do not exist.

- [ ] **Step 3: Write the implementation**

- `internal/domain/entity/payment.go`: add the three fields.
- `internal/repository/model/payment_model.go`:
  ```go
  	IdempotencyKey     string `gorm:"size:64;uniqueIndex"`
  	RequestFingerprint string `gorm:"size:64"`
  	Replayed           bool   `gorm:"not null;default:false"`
  ```
- `internal/repository/gorm/payment_repository.go`: add `FindByIdempotencyKey` (same shape as `FindByInvoiceNumber`, returning the package's not-found domain error), and make sure `toPaymentModel` / `toPaymentEntity` carry the three new fields.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/repository/gorm/ -run TestLivePayment -count=1 -v`
Expected: PASS.

Then run `go test ./internal/... -count=1` — the AutoMigrate change adds the unique index, which the live tests create fresh.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/domain/entity/payment.go internal/repository/model/payment_model.go internal/repository/gorm/payment_repository.go internal/repository/gorm/payment_live_test.go
git add internal/domain internal/repository
git commit -m "feat(payments): persist the idempotency key and fingerprint per payment"
```

---

### Task 3 (Part A): The DOKU client sends a stable key, and the usecase supplies it

**Files:**
- Modify: `internal/infrastructure/payment/gateway.go`
- Modify: `internal/infrastructure/doku/client.go`
- Modify: `internal/usecase/payment_usecase.go`
- Test: `internal/infrastructure/doku/client_test.go`
- Test: `internal/usecase/payment_usecase_test.go`

**Interfaces:**
- `payment.CreateCheckoutRequest` gains `RequestID string` — the idempotency key to send as the `Request-Id` header.
- `payment.CreateCheckoutResult` gains `Replayed bool` — true when DOKU answered 409 with the original checkout.

- [ ] **Step 1: Write the failing tests**

Add to `internal/infrastructure/doku/client_test.go`:

```go
func TestCreateCheckout_UsesTheGivenRequestID(t *testing.T) {
	var capturedHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Clone()
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/1"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-42-7",
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "qris",
		RequestID:     "SRB-42-7",
	})
	require.NoError(t, err)
	assert.Equal(t, "SRB-42-7", capturedHeader.Get("Request-Id"))
}

func TestCreateCheckout_RequiresARequestID(t *testing.T) {
	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: "https://api-sandbox.doku.com"})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-42-7",
		Amount:        10000,
		Channel:       "qris",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request id")
}

func TestCreateCheckout_Treats409WithTheOriginalBodyAsAReplay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/original"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	result, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-42-7",
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "qris",
		RequestID:     "SRB-42-7",
	})
	require.NoError(t, err, "a 409 carrying the original checkout is a replay, not a failure")
	assert.Equal(t, "https://checkout.doku.test/pay/original", result.PaymentURL)
	assert.True(t, result.Replayed)
}

func TestCreateCheckout_Treats409WithoutAResultAsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"something else"}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-42-7",
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "qris",
		RequestID:     "SRB-42-7",
	})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/infrastructure/doku/ -count=1`
Expected: FAIL — the `RequestID`/`Replayed` fields do not exist and the client still generates a random UUID.

- [ ] **Step 3: Write the implementation**

- `internal/infrastructure/payment/gateway.go`: add `RequestID string` to `CreateCheckoutRequest` and `Replayed bool` to `CreateCheckoutResult`.
- `internal/infrastructure/doku/client.go`:
  - replace `requestID := uuid.NewString()` with:
    ```go
    	requestID := req.RequestID
    	if requestID == "" {
    		return nil, fmt.Errorf("doku checkout requires a request id (the idempotency key)")
    	}
    ```
    and delete the now-unused `uuid` import.
  - after reading the response body, handle the conflict case **before** the generic non-2xx error:
    ```go
    	if resp.StatusCode == http.StatusConflict {
    		// DOKU answers a duplicate Request-Id with 409 and the original
    		// response body. That is a replay, not a failure.
    		var replay checkoutResponse
    		if err := json.Unmarshal(raw, &replay); err == nil && replay.Response.Payment.URL != "" {
    			return &payment.CreateCheckoutResult{
    				PaymentURL:        replay.Response.Payment.URL,
    				ProviderRequestID: requestID,
    				ProviderSessionID: replay.Response.Order.SessionID,
    				ExpiresAt:         expiryOf(replay, c.now(), req.ExpiryMinutes),
    				Replayed:          true,
    			}, nil
    		}
    	}
    ```
    (factor the expiry parsing into a small helper so both paths share it).
  - the `payment.CreateCheckoutRequest` built by the usecase must carry `Channel` — the client maps the slug to the DOKU channel name; if the slug is unknown, return an error instead of sending a bogus channel.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/infrastructure/doku/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/infrastructure/doku/client.go internal/infrastructure/payment/gateway.go internal/infrastructure/doku/client_test.go
git add internal/infrastructure
git commit -m "feat(payments): send a stable request id and read a 409 checkout as a replay"
```

---


**Interfaces:**
- `PaymentUsecase.CreateCheckout(ctx, booking, channel)` now derives `SRB-<bookingID>-<paymentID>` from the persisted payment row, stores it with a request fingerprint **before** calling the gateway, and passes it as `RequestID`.
- New: `PaymentUsecase.RetryCheckout(ctx, bookingID uint, channel string) (*entity.Payment, error)` — reuses the existing payment row, therefore the same key and fingerprint.


#### Steps (the client and the usecase in one commit)
- [ ] **Step 1: Write the failing tests**

Add to `internal/usecase/payment_usecase_test.go`:

```go
func TestPaymentUsecase_CreateCheckout_PersistsTheKeyBeforeCallingTheGateway(t *testing.T) {
	repo := &fakePaymentRepo{}
	secret := &fakeSecretBox{}
	gateway := &fakeGateway{url: "https://checkout.test/pay/1"}
	uc := NewPaymentUsecase(repo, &fakePaymentBookingRepo{}, nil, gateway, "sandbox", []string{"qris"}, 60, "http://localhost:5173")

	booking := &entity.Booking{ID: 42, CustomerID: 7, Total: 155000, Status: entity.BookingStatusPendingPayment}
	record, err := uc.CreateCheckout(context.Background(), booking, "qris")

	require.NoError(t, err)
	assert.Equal(t, "SRB-42-1", record.IdempotencyKey)
	assert.NotEmpty(t, record.RequestFingerprint)
	require.Len(t, gateway.requests, 1)
	assert.Equal(t, "SRB-42-1", gateway.requests[0].RequestID, "the gateway must receive the persisted key")
}

func TestPaymentUsecase_CreateCheckout_UsesANewKeyForANewAttempt(t *testing.T) {
	repo := &fakePaymentRepo{}
	gateway := &fakeGateway{url: "https://checkout.test/pay/1"}
	uc := NewPaymentUsecase(repo, &fakePaymentBookingRepo{}, nil, gateway, "sandbox", []string{"qris"}, 60, "http://localhost:5173")

	first, err := uc.CreateCheckout(context.Background(), &entity.Booking{ID: 42, Total: 155000}, "qris")
	require.NoError(t, err)
	second, err := uc.CreateCheckout(context.Background(), &entity.Booking{ID: 42, Total: 155000}, "qris")
	require.NoError(t, err)

	assert.NotEqual(t, first.IdempotencyKey, second.IdempotencyKey, "a new attempt is a new row and gets a new key")
}

func TestPaymentUsecase_RetryCheckout_ReusesTheSameKeyAndFingerprint(t *testing.T) {
	repo := &fakePaymentRepo{}
	gateway := &fakeGateway{url: "https://checkout.test/pay/1"}
	uc := NewPaymentUsecase(repo, &fakePaymentBookingRepo{}, nil, gateway, "sandbox", []string{"qris"}, 60, "http://localhost:5173")

	booking := &entity.Booking{ID: 42, Total: 155000}
	created, err := uc.CreateCheckout(context.Background(), booking, "qris")
	require.NoError(t, err)

	retried, err := uc.RetryCheckout(context.Background(), 42, "qris")
	require.NoError(t, err)
	assert.Equal(t, created.IdempotencyKey, retried.IdempotencyKey)
	assert.Equal(t, created.RequestFingerprint, retried.RequestFingerprint)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/usecase/ -run TestPaymentUsecase -count=1`
Expected: FAIL — the key is not persisted and `RetryCheckout` does not exist.

- [ ] **Step 3: Write the implementation**

In `internal/usecase/payment_usecase.go`:

- After the payment row is created (`paymentRepo.Create`), compute:
  ```go
  	key := idempotency.CollectionKey(booking.ID, record.ID)
  	fingerprint := idempotency.Fingerprint(key, idempotency.CanonicalPaymentRequest(record.InvoiceNumber, record.Amount, record.Currency, channel, uc.expiryMinutes))
  ```
  Use the canonical-string helper from the idempotency package (`idempotency.CanonicalPaymentRequest`), not a second local copy.
  Persist both on the row **before** the gateway call, with the repository method
  `UpdateIdempotency(ctx, paymentID uint, key, fingerprint string) error` (added to the
  `repository.PaymentRepository` interface, with the two existing fakes updated).
- Pass `RequestID: key` in the `payment.CreateCheckoutRequest`.
- After a successful call, set `record.Replayed = result.Replayed` and persist it with
  `MarkReplayed(ctx, paymentID uint, replayed bool) error` (also added to the interface).
- Add `RetryCheckout(ctx, bookingID, channel)`:
  - find the booking's payment row; if it is already `pending` with a checkout URL, return it unchanged (nothing to do);
  - if it is `expired`, refuse (`PAYMENT_EXPIRED`);
  - otherwise re-run the checkout with the **stored** key and fingerprint (never a new one) and update the row.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/usecase/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/usecase/payment_usecase.go internal/usecase/payment_usecase_test.go
git add internal/usecase
git commit -m "feat(payments): persist the idempotency key before the checkout and allow a safe retry"
```

---

### Task 4 (Part A): Booking creation honours an Idempotency-Key header

**Files:**
- Modify: `internal/domain/entity/booking.go`
- Modify: `internal/repository/model/booking_model.go`
- Modify: `internal/repository/gorm/booking_repository.go`
- Modify: `internal/usecase/booking_usecase.go`
- Modify: `internal/handler/booking_handler.go`
- Test: `internal/usecase/booking_prepay_test.go`, `internal/handler/booking_handler_test.go` (new)

**Interfaces:**
- `entity.Booking` gains `IdempotencyKey string`; the bookings table gains `idempotency_key varchar(64)` with a unique index (nullable, so rows without a key are allowed).
- `BookingUsecase.CreateBooking(ctx, customerID, req, idempotencyKey string)` — a non-empty key that already exists returns the existing booking.
- `BookingRepository.FindByIdempotencyKey(ctx, key string) (*entity.Booking, error)`.

- [ ] **Step 1: Write the failing tests**

- In `internal/usecase/booking_prepay_test.go`: a test that creating a booking twice with the same key returns the same booking id and creates one row; and that the same key with a different service id is rejected with `IDEMPOTENCY_KEY_REUSED`.
- In a new `internal/handler/booking_handler_test.go`: `POST /api/v1/bookings` with the header `Idempotency-Key: abc` twice returns the same booking id and creates one booking.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/usecase/ ./internal/handler/ -run "Idempot" -count=1`
Expected: FAIL — the key is not stored or honoured.

- [ ] **Step 3: Write the implementation**

- Model + entity + repository: `idempotency_key` with `uniqueIndex` (a nullable column, so existing rows stay valid), plus `FindByIdempotencyKey`.
- `BookingUsecase.CreateBooking(ctx, customerID, req, idempotencyKey string)`:
  - if the key is not empty, look it up first; if it exists **for the same customer** and the same request fingerprint, return that booking (do not create a second one); if the fingerprint differs, return the domain error `IDEMPOTENCY_KEY_REUSED`; if it belongs to another customer, also refuse (do not leak another customer's booking).
  - store the key and the fingerprint on the new booking row.
- `BookingHandler.Create`: read the `Idempotency-Key` header (trim, max 64 chars, ignore an over-long one) and pass it down. The response is the same booking shape as today, so the frontend needs no change beyond sending the header.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/usecase/ ./internal/handler/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/domain/entity/booking.go internal/repository/model/booking_model.go internal/repository/gorm/booking_repository.go internal/usecase/booking_usecase.go internal/handler/booking_handler.go
git add internal/domain internal/repository internal/usecase internal/handler
git commit -m "feat(bookings): honour an Idempotency-Key header so a double submit cannot charge twice"
```

---

### Task 5 (Part A): The frontend sends the key and can retry a failed payment

**Files:**
- Modify: `src/services/booking.service.js`
- Modify: `src/pages/booking-confirm/hooks/useBookingConfirm.js`
- Modify: `src/pages/booking-confirm/index.jsx`
- Modify: `src/pages/riwayat/components/BookingDetailModal.jsx`
- Test: `src/__tests__/pages/BookingConfirmPrepay.test.jsx`

**Interfaces:**
- `bookingService.create(data, { idempotencyKey })` sends the header `Idempotency-Key`.
- `bookingService.retryPayment(id)` → `POST /api/v1/bookings/{id}/payment/retry`.

- [ ] **Step 1: Write the failing test**

Extend `src/__tests__/pages/BookingConfirmPrepay.test.jsx`:

```jsx
it('sends one idempotency key per submission and reuses it on a retry', async () => {
  mockCreate.mockResolvedValue({ id: 80, payment: { status: 'pending', channel: 'qris', payment_url: 'https://checkout.test/pay/80' } })
  mockRedirect.mockReturnValue(true)

  renderConfirm()
  await waitFor(() => expect(screen.getByText('QRIS')).toBeInTheDocument())

  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: /Buat Pesanan & Bayar/i }))

  await waitFor(() => expect(mockCreate).toHaveBeenCalledTimes(1))
  const firstKey = mockCreate.mock.calls[0][1].idempotencyKey
  expect(firstKey).toBeTruthy()
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npx vitest run src/__tests__/pages/BookingConfirmPrepay.test.jsx`
Expected: FAIL — no idempotency key is sent.

- [ ] **Step 3: Write the implementation**

- `booking.service.js`:
  ```js
  async create(data, options = {}) {
    const response = await api.post('/api/v1/bookings', data, {
      headers: options.idempotencyKey ? { 'Idempotency-Key': options.idempotencyKey } : undefined,
    })
    return response.data.data
  },

  async retryPayment(id) {
    const response = await api.post(`/api/v1/bookings/${id}/payment/retry`)
    return response.data.data
  },
  ```
- `useBookingConfirm.js`: create the key once per submission flow — a `useRef` holding a UUID that is generated when the user first submits and reused while the same submission is retried; pass it to `bookingService.create`. On a failed submission the key stays, so pressing the button again reuses it.
- The booking detail modal: when a payment is `failed` and has no checkout URL, offer "Coba lagi pembayaran" that calls `retryPayment` and follows the returned `payment.payment_url`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `npx vitest run src/__tests__/pages/BookingConfirmPrepay.test.jsx`
Then: `npm run test:run`, `npm run lint`, `npm run build`.

- [ ] **Step 5: Commit**

```bash
git add src/services/booking.service.js src/pages/booking-confirm src/pages/riwayat src/__tests__
git commit -m "feat(booking): send an idempotency key and offer a safe payment retry"
```

---

## Part B — Withdrawal idempotency

### Task 6 (Part B): The payouts ledger

**Files:**
- Create: `internal/domain/entity/payout.go`
- Create: `internal/repository/model/payout_model.go`
- Create: `internal/domain/repository/payout_repository.go`
- Create: `internal/repository/gorm/payout_repository.go`
- Modify: `cmd/server/main.go` (AutoMigrate)
- Test: `internal/repository/gorm/payout_live_test.go`

**Interfaces:**
- `entity.Payout` with: `ID`, `BookingID`, `ProviderID`, `Amount`, `Currency`, `Channel`, `Status`, `PartnerReferenceNo`, `IdempotencyKey`, `RequestFingerprint`, `ExternalID`, `DokuInquiryReferenceNo`, `DokuReferenceNo`, `DokuReferenceNumber`, `BeneficiaryBankCode`, `BeneficiaryAccountHolderNameEncrypted`, `BeneficiaryAccountNumberEncrypted`, `BeneficiaryLast4`, `BeneficiaryFingerprint`, `Replayed`, `LastError`, `PaidAt`, timestamps.
- Statuses: `pending`, `inquiry`, `submitted`, `paid`, `failed`.
- `repository.PayoutRepository`: `Create`, `FindByID`, `FindByBookingID`, `FindByPartnerReferenceNo`, `Update`.

- [ ] **Step 1: Write the failing test**

`internal/repository/gorm/payout_live_test.go`: create a payout for booking 42, find it by booking id and by partner reference number, and assert that creating a **second** payout for the same booking fails (the unique index), and likewise for a duplicate `partner_reference_no`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/repository/gorm/ -run TestLivePayout`
Expected: FAIL — the types do not exist.

- [ ] **Step 3: Write the implementation**

The table `payouts` with `booking_id` **unique** and `partner_reference_no` **unique**; `external_id` is a numeric string column. The entity, the model, the repository interface and the GORM implementation, following the pattern of the payment repository. Register `&model.PayoutModel{}` in the AutoMigrate list.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/repository/gorm/ -run TestLivePayout -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/domain internal/repository cmd/server/main.go
git add internal/domain internal/repository cmd/server/main.go
git commit -m "feat(payouts): add the payouts ledger with one row per booking"
```

---

### Task 7 (Part B): The DOKU Sub-Account client (B2B token, HMAC-SHA512, transfer)

**Files:**
- Create: `internal/infrastructure/doku/subaccount.go`
- Test: `internal/infrastructure/doku/subaccount_test.go`

**Interfaces:**
- `doku.SubAccountClient` with `NewSubAccountClient(cfg Config) *SubAccountClient` and:
  - `TransferInquiry(ctx, req TransferInquiryRequest) (*TransferInquiryResult, error)`
  - `TransferPayment(ctx, req TransferPaymentRequest) (*TransferPaymentResult, error)`
  - `TransactionStatus(ctx, partnerReferenceNo string) (*TransactionStatusResult, error)`
  - `BalanceInquiry(ctx, profileID string) (*BalanceResult, error)`
- Signing helper: `SignSHA512(clientSecret, method, endpointURL, accessToken, body, timestamp string) string` with `stringToSign = method + ":" + endpointURL + ":" + accessToken + ":" + lowercase(hex(SHA256(minify(body)))) + ":" + timestamp`.
- Token cache: the B2B token is fetched with `POST /adv-core-api/partner/v1.0/token` (non-SNAP signed headers, body `{"grant_type":"client_credentials","valid_time":"360"}`) and cached until 60 seconds before `valid_time` expires.

**Headers on every Sub-Account call:** `Authorization: Bearer <token>`, `X-PARTNER-ID`, `X-TIMESTAMP` (ISO 8601 with timezone offset), `X-SIGNATURE` (base64 HMAC-SHA512), `X-EXTERNAL-ID` (numeric string).

**Transfer bodies:**
- inquiry: `{"partnerReferenceNo","type":"BANK_ACCOUNT","amount":{"value":"155000.00","currency":"IDR"},"channel":"BI_FAST","fromAccount","beneficiaryBankCode","beneficiaryAccountNumber","remark"}`
- payment: the same plus `"referenceNo"` (from the inquiry) and `"beneficiaryAccountName"` (from the inquiry).

**409 handling:** a `409` on either call means the same `partnerReferenceNo` was already processed. The client returns the parsed result with `Replayed = true` when the body carries a `referenceNo`; otherwise it is an error.

- [ ] **Step 1: Write the failing tests**

`internal/infrastructure/doku/subaccount_test.go`:
- the signature matches a known vector (compute the expected value with an independent HMAC-SHA512 implementation inside the test);
- the transfer call sends the four `X-` headers, with a numeric `X-EXTERNAL-ID`, and the `Authorization: Bearer` token;
- the `X-SIGNATURE` covers the body: changing the body changes the signature;
- a `409` with a `referenceNo` is returned as a replay; a `409` without one is an error;
- the token is fetched once and reused for the next call (the test server counts token requests).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/infrastructure/doku/ -run TestSubAccount -count=1`
Expected: FAIL — the client does not exist.

- [ ] **Step 3: Write the implementation**

`internal/infrastructure/doku/subaccount.go` — the client, the token cache (with a mutex), the minify + SHA-256 + hex digest helper, and the HMAC-SHA512 signer. Keep the existing `Digest`/`Sign` helpers untouched (they serve the Checkout API, which uses non-SNAP).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/infrastructure/doku/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/infrastructure/doku/subaccount.go internal/infrastructure/doku/subaccount_test.go
git add internal/infrastructure/doku
git commit -m "feat(doku): add the Sub-Account client with B2B token and SHA-512 signing"
```

---

### Task 8 (Part B): The payout usecase — one payout per booking, replay-aware

**Files:**
- Create: `internal/usecase/payout_usecase.go`
- Create: `internal/dto/payout_dto.go`
- Test: `internal/usecase/payout_usecase_test.go`

**Interfaces:**
- `PayoutUsecase.Release(ctx, bookingID uint) (*entity.Payout, error)`:
  1. load the booking: it must be `completed` (or `confirmed`) and have a `paid` payment; otherwise refuse;
  2. load the provider's payout destination (decrypting the account number with the existing `SecretBox`);
  3. **create or reuse** the payout row for that booking (unique index): the key is `PO-<bookingID>`, the fingerprint covers booking, amount, currency, bank code, account number and holder name;
  4. if the payout already exists with a different fingerprint, refuse with `PAYOUT_DESTINATION_CHANGED`;
  5. if the payout is already `submitted` or `paid`, return it unchanged — **never** transfer again;
  6. run the transfer inquiry with the stored `partnerReferenceNo`; a `409` is reconciled with `transactions-status`;
  7. run the transfer payment with the inquiry's `referenceNo` and the same `partnerReferenceNo`; a `409` means the transfer was already submitted → mark `submitted` and reconcile, never retry as a new transfer;
  8. store the provider references and the state.
- `PayoutUsecase.ApplyTransferStatus(ctx, partnerReferenceNo, status string)` — the webhook path: `submitted` → `paid` is idempotent; a `paid` payout is never changed again.

- [ ] **Step 1: Write the failing tests**

`internal/usecase/payout_usecase_test.go` with fakes for the payout repository, the booking repository, the payment repository, the payout-destination repository, the cipher and the Sub-Account client:
- releasing twice creates **one** payout row and calls the provider once;
- a provider `409` on the transfer payment marks the payout `submitted` and does **not** call the provider again;
- a payout already `submitted` is returned unchanged without any provider call;
- the same booking with a different destination is refused (`PAYOUT_DESTINATION_CHANGED`);
- `ApplyTransferStatus` on an already-paid payout changes nothing;
- the amount paid is the booking's service amount (`sub_total`), and the platform fee is not included.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/usecase/ -run TestPayoutUsecase -count=1`
Expected: FAIL — the usecase does not exist.

- [ ] **Step 3: Write the implementation**

`internal/usecase/payout_usecase.go` with the flow above. The provider call goes through a narrow interface so the tests can fake it:

```go
// TransferGateway is the money-moving boundary of the payout usecase.
type TransferGateway interface {
	TransferInquiry(ctx context.Context, req TransferInquiryRequest) (*TransferInquiryResult, error)
	TransferPayment(ctx context.Context, req TransferPaymentRequest) (*TransferPaymentResult, error)
	TransactionStatus(ctx context.Context, partnerReferenceNo string) (*TransactionStatusResult, error)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/usecase/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/usecase/payout_usecase.go internal/usecase/payout_usecase_test.go internal/dto/payout_dto.go
git add internal/usecase internal/dto
git commit -m "feat(payouts): release a Mitra payout exactly once per booking"
```

---

### Task 9 (Part B): The admin release endpoint and the transfer webhook

**Files:**
- Create: `internal/handler/payout_handler.go`
- Modify: `cmd/server/main.go` (routes)
- Test: `internal/handler/payout_handler_test.go`

**Interfaces:**
- `POST /api/v1/admin/bookings/{id}/payout` — admin only; releases the payout; returns the payout state. A second call returns the same payout without moving money.
- `POST /api/v1/payments/doku/transfer-webhook` — public; DOKU's transfer notification; signature-verified with the Sub-Account scheme (the same signature the client uses, over the notification path and body); idempotent.

- [ ] **Step 1: Write the failing tests**

`internal/handler/payout_handler_test.go`: an admin can release a payout and gets the payout state; a non-admin gets 403; the webhook with a valid signature moves the payout to `paid`; the same webhook twice changes nothing.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/handler/ -run TestPayoutHandler -count=1`
Expected: FAIL.

- [ ] **Step 3: Write the implementation**

The handler, the routes (admin group and the public webhook next to the existing DOKU webhook), and the wiring in `main.go`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/handler/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/handler/payout_handler.go internal/handler/payout_handler_test.go cmd/server/main.go
git add internal/handler cmd/server/main.go
git commit -m "feat(payouts): add the admin release endpoint and the transfer webhook"
```

---

### Task 10 (Part B): Contract and documentation

**Files:**
- Modify: `serabutin-technical-documentation/api-contracts/openapi.yaml`
- Create: `serabutin-technical-documentation/spec/018.idempotency-money-movements.md`
- Modify: `serabutin-technical-documentation/research/2026-10-18-doku-payment-gateway-assessment.md` (rev. 6)
- Modify: `serabutin-technical-documentation/spec/handoff/2026-10-31-prepay-doku-checkout.md`

- [ ] **Step 1: Add the OpenAPI paths**

`POST /api/v1/bookings/{id}/payment` (customer, retry the checkout), `POST /api/v1/admin/bookings/{id}/payout` (admin), `POST /api/v1/payments/doku/transfer-webhook` (public), and the `Idempotency-Key` header on `POST /api/v1/bookings`. Add the `PayoutResponse` schema and document the `409` replay semantics.

- [ ] **Step 2: Write spec 018**

Sections: Prompt, Goal, Why (DOKU's rule and our random-UUID defect), Codebase Analysis, Logic Changes (the key convention, the replay table, the fingerprint), Code Changes (per file), Pseudocode (the two flows), Test Simulation (the Review Focus cases), Manual Testing Plan, and Open items.

- [ ] **Step 3: Update the research document**

Rev. 6: record that DOKU's idempotency key is the `Request-Id` header and that a duplicate is answered with **409 and the original body**; that the Sub-Account transfer API deduplicates on `partnerReferenceNo` (409 on duplicate) and requires a numeric `X-EXTERNAL-ID`; and that we derive both keys from persisted rows.

- [ ] **Step 4: Update the handoff**

Add the idempotency work to "What I did"; add to "What's pending": the automatic release trigger and the settlement gate, the DOKU fee policy for withdrawals (DOKU does not publish Sub-Account pricing, so the Mitra-absorbs-DOKU-fee policy cannot be computed yet — the interim rule pays the service amount and leaves the transfer fee to serabutin), and the DOKU refund path.

- [ ] **Step 5: Validate and commit**

```bash
cd serabutin-technical-documentation
python3 -c "import yaml;d=yaml.safe_load(open('api-contracts/openapi.yaml'));print(len(d['paths']))"
git add -A
git commit -m "docs(payments): idempotency for every money movement — spec, contract, research rev 6"
```

---

## Review Focus

The inputs most likely to bite someone using this, and the behaviour each must produce:

1. **The provider call times out after DOKU already created the invoice** → the retry must return the *original* checkout, not create a second one.
2. **The customer double-clicks "Buat Pesanan & Bayar"** → one booking, one payment, one invoice.
3. **Two release jobs run at once for the same booking** → one payout row, one transfer, one payment to the Mitra.
4. **A retry carries a different amount or destination** → rejected with `IDEMPOTENCY_KEY_REUSED`, never silently applied.
5. **DOKU answers 409** → a replay when the body carries the original result, an error otherwise; for a transfer, "already submitted" and reconciled, never re-sent.
6. **The process restarts between the inquiry and the transfer** → the payout row keeps its key and reference numbers, so the resumed run continues the same transfer.

## Execution Notes

- The whole backend suite (`go test ./...`) must be green after every task; the live tests need Docker.
- Keep one commit per task so a reviewer can gate each one.
- Part A can be released on its own; Part B needs the hub sub-account to move real money, but the code and its tests ship first.

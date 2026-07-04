# Fixes Report

## What was fixed

1. **Fix 1 (Critical)** — `internal/seed/reviews.go`: Replaced Chinese character `满意` with `puas` in review comment.
2. **Fix 2 (Important)** — `internal/seed/bookings.go`: Made `platformFee` a parameter instead of hardcoded `int64(5000)`. Updated `Seed` signature in `seed.go`, call in `cmd/seed/main.go`, and tests in `bookings_test.go`.
3. **Fix 3 (Important)** — `internal/seed/users.go`: Replaced fragile WhatsApp number truncation with clean 15-digit Indonesian number generation.
4. **Fix 4 (Important)** — `internal/seed/bookings.go`: Booking address now uses a street name pool instead of the customer's city as street.
5. **Fix 5 (Important)** — `internal/seed/reviews.go`: Rating range expanded from 3-5 to 1-5 for all 4 rating dimensions; removed dead clamping code.
6. **Fix 6 (Important)** — `internal/seed/seed.go`: Added per-100-record progress logging in service and booking insertion loops.
7. **Fix 7 (Minor)** — `internal/seed/reviews.go`: Reviews now have per-review `CreatedAt`/`UpdatedAt` spacing (1 minute apart).
8. **Fix 8 (Minor)** — `internal/seed/seed.go`: Replaced inefficient `savedUsers` reload with direct use of populated `users` slice.

## Test Results

```
go test ./internal/seed/ -v -count=1
--- PASS: TestGenerateBookings_ReturnsCorrectCount (0.00s)
--- PASS: TestGenerateBookings_AllCompleted (0.00s)
--- PASS: TestRandomItemReturnsItemFromSlice (0.00s)
--- PASS: TestRandomIntWithinRange (0.00s)
--- PASS: TestGenerateReviews_ReturnsCorrectCount (0.00s)
--- PASS: TestGenerateReviews_MapsBookingFields (0.00s)
--- PASS: TestGenerateServices_ReturnsCorrectCount (0.01s)
--- PASS: TestGenerateServices_AllLinkedToProviders (0.00s)
--- PASS: TestGenerateServices_HasImages (0.00s)
--- PASS: TestGenerateUsers_ReturnsCorrectCount (0.83s)
--- PASS: TestGenerateUsers_UniqueEmails (0.78s)
--- PASS: TestGenerateUsers_UniqueWhatsApp (0.75s)
--- PASS: TestGenerateUsers_HashedPasswords (0.76s)
--- PASS: TestGenerateUsers_ProviderCount (0.75s)
PASS
ok  	github.com/serabutin/serabutin-id-backend/internal/seed	4.466s
```

All 14 tests passed.

## Build Results

- `go build ./cmd/seed/` — OK
- `go build ./...` — OK

# Task 1 Report: Static Data Pool

## What I Implemented

Created `internal/seed/` package with:
- `data.go` — Static data slices for Indonesian locale:
  - `firstNames` (51 entries)
  - `lastNames` (26 entries)
  - `cities` (25 entries)
  - `provinces` (18 entries)
  - `categories` (10 entries)
  - Generic helper `randomItem[T any](rng, items)` — picks random element from slice
  - Helper `randomInt(rng, min, max)` — returns random int in [min, max] inclusive
- `data_test.go` — Two unit tests covering both helper functions

## Test Results

```
=== RUN   TestRandomItemReturnsItemFromSlice
--- PASS: TestRandomItemReturnsItemFromSlice (0.00s)
=== RUN   TestRandomIntWithinRange
--- PASS: TestRandomIntWithinRange (0.00s)
PASS
ok  	github.com/serabutin/serabutin-id-backend/internal/seed	0.449s
```

## Files Changed

- `internal/seed/data.go` (created, 69 lines)
- `internal/seed/data_test.go` (created, 20 lines)

## Self-Review Findings

- Code follows Go conventions: unexported vars/funcs, no comments, proper `math/rand` usage with explicit `*rand.Rand` parameter (not global default).
- Tests use `assert.Contains`, `assert.GreaterOrEqual`, `assert.LessOrEqual` as appropriate.
- `randomItem` is generic (`[T any]`) — idiomatic Go; no issues.
- `randomInt` handles edge case `min == max` correctly (returns `min`).
- CRLF warnings on commit are cosmetic (Windows environment, `.gitattributes` may need `* text=auto`).

## Issues or Concerns

None.

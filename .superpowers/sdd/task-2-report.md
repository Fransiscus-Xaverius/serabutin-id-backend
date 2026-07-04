# Task 2: User Generator — Report

## Status: DONE

| Item | Detail |
|------|--------|
| **Commit** | `ba68437` — feat(seed): add user generator (200 providers + 800 customers) |
| **Test results** | 7/7 PASS (2 data helpers + 5 user generator tests) |
| **Files** | `internal/seed/users.go` (impl), `internal/seed/users_test.go` (tests) |

## Summary

- `generateUsers(bcryptCost int)` generates 1000 users: first 200 as `provider`, remaining 800 as `customer`.
- Emails are unique via `firstName.lastName.{index}@email.com` pattern.
- WhatsApp numbers are unique via randomized digit format truncated to 16 chars.
- Passwords are bcrypt-hashed using cost 4 (fast test execution).
- Uses shared helpers `randomItem`, `randomInt`, and data slices from `data.go`.
- All 7 tests in `internal/seed` pass in ~4.2s.

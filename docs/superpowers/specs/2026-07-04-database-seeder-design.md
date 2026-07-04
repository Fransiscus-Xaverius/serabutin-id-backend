# Database Seeder — Design Spec

## Overview

A standalone CLI tool that populates the serabutin.id database with 1000 users, 1000 services, 1000 bookings, and 1000 reviews of realistic test data. Runs separately from the main server, targets an empty database after migration.

## Approach

**Go-based standalone CLI using GORM directly** (`cmd/seed/main.go`). Reuses existing `config.Load()`, `database.NewPostgresDB()`, and GORM model structs. All seeding happens inside a single transaction — all-or-nothing.

## Data Generation Strategy

Users (200 providers + 800 customers) seeded first, then services (providers each list ~5 services), then bookings (customers book services from random providers), then reviews (one per completed booking).

All data uses realistic Indonesian locale (names, cities, provinces, phone prefixes) to match serabutin.id's market.

Categories: House Cleaning, AC Service, Plumbing, Electrical, Painting, Gardening, Catering, Photography, Massage, Tutoring.

All 1000 bookings are completed (each gets a review). Dates spread across the past 3 months.

Reviews: one per completed booking (1000 total). Four rating dimensions per review.

## File Structure

```
cmd/seed/main.go           — entry point, config + DB setup, orchestration
internal/seed/
  seed.go                  — orchestrator, runs generators in order within a single tx
  users.go                 — generate & bulk-insert 1000 users, bcrypt at configured cost
  services.go              — generate & bulk-insert 1000 services + images + add-ons
  bookings.go              — generate & bulk-insert 1000 bookings + add-ons
  reviews.go               — generate & bulk-insert 1000 reviews
  data.go                  — static pools: names, cities, provinces, categories, etc.
```

## Data Relationships

| Entity     | FK references                     |
|------------|-----------------------------------|
| service    | provider_id → users.id (providers) |
| booking    | customer_id → users.id (customers) |
|            | service_id → services.id           |
|            | provider_id → services.provider_id |
| review     | booking_id → bookings.id (completed only) |
|            | service_id → bookings.service_id    |
|            | customer_id → bookings.customer_id  |
|            | provider_id → bookings.provider_id  |

## Key Details

- **Password**: all users use `testing123`, bcrypt at configured cost (config `BCRYPT_COST`), unique hash per user
- **Batch insert**: 100 records per batch to keep memory low
- **Progress logging**: timestamped every 100 records per entity, summary at the end
- **Transaction safety**: single GORM transaction, full rollback on any error
- **Not re-runnable**: will fail on unique constraints (email, whatsapp); intended for fresh databases only
- **Prices**: realistic ranges (50k–500k IDR for services, platform fee from config)
- **Schedule**: all bookings have random past dates in the last 90 days, all completed
- **Review ratings**: all four dimensions (overall, communication, quality, value) 1–5 scale

## Progress & Error Handling

- Every 100 records: `"Seeded 300/1000 users..."` with elapsed time
- On error: `log.Fatal` with the error, transaction is rolled back
- On success: `"Seeded 1000 users, 1000 services, 1000 bookings, X reviews in Y.Ys"`

## Non-goals

- No cleanup/truncate command (assumes fresh DB)
- No idempotent re-run (upserts would add complexity without value for a dev seeder)
- No configurable record counts (can be added later if needed)

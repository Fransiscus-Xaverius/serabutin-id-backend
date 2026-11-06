# Squad State — DOKU integration (option 3)
Handoff file for further agents. **No commits/pushes.** Preserve unrelated work.

## Status
- degraded: phase-1-smoke (browser installed; rerun in progress, background)
- decisions: Q1=A (admin smoke now), Q2=A (keep uncommitted payout baseline), Q3=A+C, Q4=mock/tests (live DOKU=Needs you), Q5=no commits/preserve work/careful with docker
- PRD: `serabutin-id-backend/PRD-doku-payment.md`
- Old admin squad state preserved: `serabutin-id-backend/squad-state.md` (all done, do not clobber)

## Environment (local, currently up as of 2026-11-05)
- Frontend: `http://localhost:5173` (container `serabutin-frontend`)
- ID backend: `http://localhost:3000` (container `serabutin-api`, rebuilt with uncommitted payout changes; uses `PAYOUT_ENCRYPTION_KEY` set at bring-up)
- Chat: `http://localhost:8080` (container `serabutin-chat`) + chat infra containers
- DB/Redis: backend `serabutin-postgres`/`serabutin-redis`; chat compose postgres/redis
- Smoke test admin (local test): `squad-admin-smoke-1793887582@test.com` / `TestPass123!` (admin, id 72). See smoke artifacts.
- Prior smoke attempt (blocked): `serabutin-id-frontend/test-screenshots/admin-smoke-20261105T140654Z/`

## DOKU payment flow (confirmed)
- **Yes (real DOKU flow):** backend creates/posts a checkout request to DOKU (`CreateCheckout`), returns checkout/payment URL/session; payment outcome is confirmed via **DOKU webhook/notification** (`POST /api/v1/payments/doku/webhook` → `PaymentUsecase.ApplyNotification`). Mock mode has local succeed/expire simulation endpoints; payout (disbursement) is a separate flow that reconciles via DOKU status API.

## Baseline evidence (2026-11-05, with uncommitted payout changes)
- `cd serabutin-id-backend && go test ./internal/usecase -run 'Payout|payout' -count=1` → ok (0.262s)
- `cd serabutin-id-backend && go test ./internal/handler -run 'Payout|payout' -count=1` → ok (0.019s)
- `cd serabutin-id-backend && go test ./internal/repository/gorm -run 'Payout|payout' -count=1` → ok (33.648s)
- Uncommitted: 7 payout files (read-only X-EXTERNAL-ID uses persisted `payout_external_id_seq`, `NextReadExternalID`/`EnsurePayoutExternalIDSequence`, read band `1,000,000,000`). **Gap A1:** `EnsurePayoutExternalIDSequence` not wired in `cmd/server/main.go`.

## Backlog (fix after DOKU work)
- [ ] **Admin smoke FAIL (Bugs/major, 2026-11-05): Keamanan Chat proxy 502** — `/admin/keamanan-chat` (`page-admin-chat`) renders but `GET http://localhost:5173/api/v1/admin/chat/conversations?page=1&limit=20` returns **502 Bad Gateway** (“chat directory unavailable”; “Coba lagi” persists). Stack was up (frontend 5173, backend 3000, chat 8080). Likely ID-backend admin chat proxy ↔ chat service/admin endpoint or service-token/config path (Caddy→ID backend→chat). Evidence: `serabutin-id-frontend/test-screenshots/admin-smoke-rerun-20261105T141956Z/REPORT.md`, `tc-admin-keamanan-chat.png/.a11y.txt`, `report.json`. Repro: login as admin, open Keamanan Chat.

## Sub-specs
- [x] Q1 — Admin smoke test rerun, status: done, attempts: 1, last: bugs (13/14 pass; see Backlog)
- [x] A1 — Wire payout read-external-id sequence at startup, status: done, attempts: 1, last: ok (cmd/server main.go calls EnsurePayoutExternalIDSequence after payout fingerprint index)
- [x] A2 — Verify payout build/tests after wiring, status: done, attempts: 1, last: ok (`go build ./...` ok; payout usecase/handler/repository/gorm tests ok)
- [x] A3 — Payout read-ID/attempt correctness review, status: done, attempts: 1, last: ok (read IDs from persisted monotonic sequence, attempt saved pre-call for throttle/observability; money IDs unchanged payoutID*10+step; no extra tests needed)
- [x] B1 — Payment checkout mock E2E/idempotency verification (backend), status: done, attempts: 1, last: ok (added handler tests: GetPayment participant, webhook SUCCESS applies, mock succeed/expire happy+unknown; existing usecase/handler payment tests pass)
- [x] B2 — Frontend/backend checkout+retry contract alignment, status: done, attempts: 1, last: ok (paths match `POST /api/v1/bookings/:id/payment/retry` + GET payment; frontend booking service/prepay/modal tests pass: 25 tests; backend retry-related tests pass)
- [x] B3 — Webhook/notification mode gating & verification behavior, status: done, attempts: 1, last: ok (mock succeed/expire gated to mock mode + happy paths verified; webhook verifies via gateway, invalid signature→401, unknown invoice→200 IGNORED, success applies; mock gateway VerifyNotification unsupported so DOKU webhook path is not for mock-mode simulation)

Rule: disjoint sub-specs may parallelize only when file ownership is clear; A before B. Max 3 attempts per sub-spec before human gate. User approved: proceed B1-B3 as drafted; backlog Keamanan Chat 502 after DOKU.
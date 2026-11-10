# Squad State — DOKU integration (option 3) — HISTORICAL

> **SUPERSEDED 2026-11-08.** Payout uses Kirim DOKU SNAP; WaaS statements below are
> historical. See `serabutin-technical-documentation/spec/handoff/2026-11-08-doku-payout-snap-migration.md`.
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

## Session 2026-11-08 — DOKU contract alignment (skill: serabutin-technical-documentation/skills/doku-payment-gateway)

Status: C4 (below) done; C5 sandbox-webhook blocked on dashboard access; no commits (preserve-work rule stands).

- [x] C1 — Checkout contract fixes (silent failures, skill ref 01/07): routing key `additionalInfo`→`additional_info` (snake_case) with `account.id` + optional `split_rule_id` field; `DOKU_HUB_ACCOUNT_ID` must match `^SAC-\d{4}-\d{13}$` or checkout fails loudly (was a documented silent failure); `invoice_number` caps 64/30(card); `callback_url_cancel` sent. Evidence: `go test ./internal/infrastructure/doku -count=1` ok.
- [x] C2 — Webhook contract fixes (skill ref 06): ack body is now `{"responseCode":"2000000","responseMessage":"Successful"}` for applied/ignored/duplicate on both webhooks; replay guard ±300s (`doku.NotificationTimestampDrift`) in payment VerifyNotification + transfer webhook; unparsable timestamp fails closed. Config: `DOKU_OVERRIDE_NOTIFICATION_URL` https + ≤64 chars; `DOKU_NOTIFICATION_PATH` absolute path (trimmed in Load). Evidence: `go test ./internal/handler ./internal/infrastructure/config -count=1` ok.
- [x] C3 — Transfer webhook scheme realigned (skill ref 06; was self-declared ASSUMPTION with bearer+SHA-512): the handler accepts both documented families — Checkout `Client-Id/Request-Id/Request-Timestamp/Signature` (HMAC-SHA256 over `/api/v1/payments/doku/transfer-webhook` + digest; wrong client id 401) and Sub-Account V2 `X-PARTNER-ID/X-TIMESTAMP/X-EXTERNAL-ID` (DOKU OpenAPI `sac-v2-all-api`, `transferNotification`; no signature published, so partner id + 300s freshness are enforced). Single-family rejection would 401 every real WaaS transfer callback. `NewPayoutReleaseHandler(payouts, clientID, secretKey, releaseErr)` signature changed. Evidence: handler tests ok.
- [x] C4 — Partner token host + evidence checks vs official docs (developers.doku.com, .md pages, accessed 2026-11-08): channel enum audit — all 20 code values match `supported-payment-methods.md` (code's `EMONEY_SHOPEE_PAY` is right; skill ref 01 table has a typo); Sub-Account V2 OpenAPI confirms payout field names `type/channel/fromAccount/remark` (skill ref 05 is the older Kirim DOKU shape — do NOT "fix" code to match it); status codes 00/03/04/05/06 already match. Partner token host is configurable via `DOKU_PARTNER_BASE_URL` (empty = `DOKU_BASE_URL`; register script got `--token-base-url` with the same fallback). Evidence: `api-sandbox.doku.com/adv-core-api/partner/v1.0/token` answers `400 "Partner is not recognized"` (endpoint reachable, WaaS not activated), while `api-uat.doku.com` times out from this network — so the default stays on the reachable host. `go test ./internal/infrastructure/doku -count=1` ok (new `TestSubAccount_TokenUsesThePartnerBaseURL`).
- [ ] C5 — Real sandbox webhook + hub registration: BLOCKED. `scripts/doku-sandbox-register.mjs` against api-uat.doku.com → connect timeout from this network (DNS resolves; host unreachable); api-sandbox token endpoint answers 400 "Partner is not recognized". Needs: WaaS/partner activation for the merchant + reachable api-uat host; then register hub, fill `DOKU_HUB_ACCOUNT_ID`/`DOKU_HUB_PROFILE_ID`/`DOKU_SOURCE_ACCOUNT`, set the notification URL in the DOKU dashboard (tunnel total URL ≤64 chars; ngrok rejected by DOKU), pay a VA BCA checkout, watch `/api/v1/payments/doku/webhook`.
- QRIS: Checkout channel `QRIS` inactive for BRN-0266-1792844429792 — mixed-channel checkout returns SUCCESS but DOKU silently drops QRIS from `payment_method_types`; QRIS-only 400 `PAYMENT CHANNEL IS INACTIVE`. Senior claims QRIS enabled → likely under Direct API QRIS SNAP (separate product; not implemented in this repo). Decision needed: activate QRIS Checkout vs build SNAP integration. `.env` uses `DOKU_PAYMENT_METHODS=dana,va_bca` meanwhile.
- Video: `recording/output/t31-doku-sandbox-story.mp4/.webm` (2026-11-08) — sandbox VA BCA story, honest labels; old mock-mode `t30-doku-story.*` kept as archive.
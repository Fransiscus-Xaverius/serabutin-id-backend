# PRD — DOKU integration (option 3: payout first, then payment checkout)

## Purpose
Complete and verify DOKU integrations across the Serabutin stack, starting with the in-progress **payout/disbursement** work, then stabilizing **customer payment checkout** end-to-end in mock/testable mode. Live DOKU sandbox/production calls are **Needs you** unless credentials/env are explicitly provided and safe to use.

## Repos in scope
- `serabutin-id-backend` (primary: payout + payment gateway/usecases/handlers/config)
- `serabutin-id-frontend` (checkout initiation/retry/payment status UX as needed)
- `serabutin-backend-chat` (only if impacted; expected no)

## Baseline / constraints
- Continue last agent’s uncommitted payout changes (Q2=A): read-only DOKU X-EXTERNAL-IDs via persisted sequence `payout_external_id_seq` (`NextReadExternalID`, `EnsurePayoutExternalIDSequence`, read band `1,000,000,000`).
- Evidence (2026-11-05): `go test ./internal/usecase -run 'Payout|payout'` ok (0.262s), `./internal/handler -run 'Payout|payout'` ok (0.019s), `./internal/repository/gorm -run 'Payout|payout'` ok (33.648s) with uncommitted changes.
- No commits/pushes. Preserve unrelated work. Edits/tests/builds/local services allowed; careful with docker (backend rebuild includes uncommitted changes).
- Verification default: **mock mode + unit/integration tests**. Real DOKU/sandbox = `Needs you`.

## Track A — DOKU payout/disbursement (first)
### Goals
1. Finalize the read X-EXTERNAL-ID sequence refactor so it works at runtime (not just tests).
2. Keep payout release/reconcile/redrive/status-sync behavior correct (per-step money IDs `payoutID*10+step`, read IDs from persisted monotonic sequence, re-drive only on DOKU not-found as previously implemented, guarded status moves, worker gating).
3. Ensure startup/runtime creates the sequence when needed.

### Known gaps / tasks
- **A1**: Wire `EnsurePayoutExternalIDSequence` into startup/migration path (currently defined in `internal/repository/gorm/payout_repository.go` but not called from `cmd/server/main.go`). Add a safe idempotent call at server init.
- **A2**: Verify full payout-related test coverage still passes after wiring; add/adjust tests if startup/sequence behavior is observable/testable.
- **A3**: Review `NextReadExternalID`/read-attempt interaction (attempt saved before call; sequence supplies ID) for correctness and no reuse/collision.
- **A4**: Run `go build ./...` (backend) and relevant packages; document any environment-only failures.

### Acceptance
- Payout unit/handler/repository tests pass.
- Backend builds.
- Sequence is ensured idempotently on startup; read-only DOKU calls obtain IDs from the sequence without colliding with money-moving IDs.

## Track B — DOKU payment checkout (after A)
### Goals
1. Verify/fix **end-to-end mock** booking → checkout creation/retry → notification/webhook handling → payment/booking state transitions.
2. Address concrete gaps (C, to be confirmed; initial candidates below).

### Candidate gaps (confirm/add)
- **B1**: End-to-end mock flow tests around `CreateCheckout`/`RetryCheckout`/notification applying (including idempotency/replay, expired/pending behavior) — confirm coverage vs product flow.
- **B2**: Frontend↔backend contract for checkout/retry (`/payment/retry` path/shape) matches and is covered (recent frontend commits touched this).
- **B3**: Webhook/notification routes and signature/verification behavior are correct for mock vs real modes; mock-only endpoints gated appropriately.
- **B4**: Error/UX states (missing checkout link, retry safety, failed checkout) behave as intended.
- **B5 (if needed)**: Channel/method mapping and config validation (`DOKU_MODE`, payment methods, notification path/override) — sandbox/live specifics = `Needs you`.

### Acceptance
- Payment/booking checkout tests pass (backend + relevant frontend tests).
- Mock-mode checkout flow is coherent and idempotent; retries reuse the intended key/fingerprint semantics.
- Any live DOKU verification documented as `Needs you` with exact steps.

## Smoke test (Q1=A) status
- Targeted admin smoke test attempted via site-test/squad approach. Stack brought up locally (frontend 5173, backend 3000, chat 8080 healthy). Test admin created/promoted: `squad-admin-smoke-1793887582@test.com`.
- **Blocked (Needs you, env):** Playwright MCP connected but `chrome-for-testing` browser binary missing (`/home/frans/.cache/ms-playwright/chromium-1252/chrome-linux64/chrome`). Suggested fix: `npx @playwright/mcp install-browser chrome-for-testing`, then re-run.
- Artifacts: `serabutin-id-frontend/test-screenshots/admin-smoke-20261105T140654Z/` (`test-cases.md`, `REPORT.md`, `report.json`).

## Out of scope / Needs you
- Installing browser binary / rerunning browser smoke test without approval.
- Real DOKU sandbox/production credentials, hub/source/profile IDs, external webhook reachability/tunnel, and live money movement.
- Commits/pushes.

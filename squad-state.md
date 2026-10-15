# Squad State — Admin console v1 (specs 002–014)

Tracker: this file (no external issue tracker configured).
Target repos: `serabutin-id-backend` (tickets 1–2, 4–13 backend slices), `serabutin-id-frontend` (UI slices of 3–11, ticket 13).

- [x] 1 — User suspend flag + login block, status: done, attempts: 0, last: ok
- [x] 2 — Admin audit trail, status: done, attempts: 0, last: ok
- [x] 3 — Admin Users page, status: done, attempts: 1, last: ok
- [x] 4 — Jasa moderation queue, status: done, attempts: 0, last: ok
- [x] 5 — Bookings ops, status: done, attempts: 1, last: ok
- [x] 6 — Reviews/reports queue, status: done, attempts: 0, last: ok
- [x] 7 — Dashboard stats, status: done, attempts: 0, last: ok
- [x] 8 — Chat safety, status: done, attempts: 1, last: ok
- [x] 9 — Broadcasts, status: done, attempts: 0, last: ok
- [x] 10 — CMS + categories, status: done, attempts: 0, last: ok
- [x] 11 — Finance overview + reimbursements, status: done, attempts: 0, last: ok
- [x] 12 — Kill switch, status: done, attempts: 0, last: ok
- [x] 13 — admin.serabutin.id routing, status: done, attempts: 0, last: ok

Phase 3 — bounty review trio: done. Referee ruling: 13/13 kept (7 at claimed severity, 7 downgraded one grade — see fix list). Zero killed.
Fix list (Engineer dispatched once for all):
- F1 HIGH suspend must revoke sessions: Authenticate/RequireAdmin must reject suspended users; Suspend must blacklist tokens via RevokeAllUserTokens (exists, unused); suspended admin loses /admin access.
- F2 HIGH introspect must return Active:false for suspended users (chat exchange then blocks fresh mints).
- F3 HIGH WS/chat must enforce block/suspend on live sessions (re-introspect/kick or per-message check in SendMessage); document chosen mechanism.
- F4 MED audit TargetID: fall back to Params("module") and record new state for kill-switches; record provider/period for payouts/mark.
- F5 MED broadcast instant path: idempotency/retry for stranded `sending` rows (requeue via ClaimDue or admin retry endpoint); document duplicate-post behavior.
- F6 MED clamp Page/Limit in booking + service ListForAdmin (match user repo: page<1→1, limit>50→20); reject negative page.
- F7 MED force-cancel atomicity: wrap Create+UpdateCancel in a transaction (or unique guard) so crash/concurrency cannot orphan/duplicate rows.
- F8 MED role guards: block self-suspend; require actor != target for suspend/demote of admins; last-admin guard on demote/suspend.
- F9 LOW introspect fail-open: accepted tradeoff, add distinguishing signal or doc note only (no behavior change required).
- F10 LOW chat conversation counts: single GROUP BY query instead of per-row COUNT.
- F11 LOW reimbursement duplicate guard: reject new claim when a pending claim exists for the same booking.
- F12 LOW payout mark: validate provider has earnings in period (or surface marks visibly); no phantom marks.
- F13 LOW broadcast reason: add optional reason to compose/cancel and persist to audit, or bless the exception in audit docs.
Re-verify: all 13 re-verified (F7 race re-fixed + live-DB concurrency test green).
Phase 4 — final rundown: in progress.

Rule: tickets touching `admin_handler.go`/middleware run sequentially; disjoint ones may parallelize.

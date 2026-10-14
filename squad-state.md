# Squad State — Admin console v1 (specs 002–014)

Tracker: this file (no external issue tracker configured).
Target repos: `serabutin-id-backend` (tickets 1–2, 4–13 backend slices), `serabutin-id-frontend` (UI slices of 3–11, ticket 13).

- [x] 1 — User suspend flag + login block, status: done, attempts: 0, last: ok
- [x] 2 — Admin audit trail, status: done, attempts: 0, last: ok
- [ ] 3 — Admin Users page, status: in-progress (QA bugs, attempt 1), attempts: 1, last: bugs
- [ ] 4 — Jasa moderation queue, status: pending, attempts: 0, last: —
- [ ] 5 — Bookings ops, status: pending, attempts: 0, last: —
- [ ] 6 — Reviews/reports queue, status: pending, attempts: 0, last: —
- [ ] 7 — Dashboard stats, status: pending, attempts: 0, last: —
- [ ] 8 — Chat safety, status: pending, attempts: 0, last: —
- [ ] 9 — Broadcasts, status: pending, attempts: 0, last: —
- [ ] 10 — CMS + categories, status: pending, attempts: 0, last: —
- [ ] 11 — Finance overview + reimbursements, status: pending, attempts: 0, last: —
- [ ] 12 — Kill switch, status: pending, attempts: 0, last: —
- [ ] 13 — admin.serabutin.id routing, status: pending, attempts: 0, last: —

Rule: tickets touching `admin_handler.go`/middleware run sequentially; disjoint ones may parallelize.

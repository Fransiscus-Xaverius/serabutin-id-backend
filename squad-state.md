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
- [ ] 9 — Broadcasts, status: in-progress (built, awaiting QA), attempts: 0, last: —
- [ ] 10 — CMS + categories, status: pending, attempts: 0, last: —
- [ ] 11 — Finance overview + reimbursements, status: pending, attempts: 0, last: —
- [ ] 12 — Kill switch, status: pending, attempts: 0, last: —
- [ ] 13 — admin.serabutin.id routing, status: pending, attempts: 0, last: —

Rule: tickets touching `admin_handler.go`/middleware run sequentially; disjoint ones may parallelize.

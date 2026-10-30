# Idempotency for every money movement — design

> Date: 2026-10-31 · Status: approved for implementation
> Related: `spec/016.payments-prepay-doku.md`, `spec/017.payout-bank-details.md`, `research/2026-10-18-doku-payment-gateway-assessment.md` (rev. 4)

## 1. Context

Two operations move money, and both talk to DOKU:

| Operation | DOKU surface | Status today |
|---|---|---|
| **Collection** — customer pays serabutin | `POST /checkout/v1/payment` (DOKU Checkout) | implemented |
| **Withdrawal** — serabutin pays the Mitra | `POST /sub-account/v2.0/transfer-inquiry` + `/transfer-payment` | **not implemented** |

DOKU's own [Idempotency Request](https://developers.doku.com/get-started-with-doku-api/idempotency-request) page states the rule plainly:

> We make `idempotency key` in our system is `Request ID`. […] Every API Request must send unique `request ID`. If something happen in Merchant Side and merchant can't get response, then merchant send request again with same request ID and same Request Body, DOKU system will send the same Response body with HTTP Status Code 409 Conflict.

Two facts follow, and both are load-bearing:

1. **The idempotency key is the `Request-Id` header.** It must be *stable across retries*, not random per attempt.
2. **A duplicate is answered with HTTP 409 and the original response body** — not with an error page. A 409 that carries `response.payment.url` is a *replay*, and our client must read it as success.

For the Sub-Account V2 transfer API the key is `partnerReferenceNo` (unique per request, `409 Duplicate partnerReferenceNo`), plus the `X-EXTERNAL-ID` header, which must be a **numeric** string, unique within the day.

**Our defect:** `internal/infrastructure/doku/client.go` generates `requestID := uuid.NewString()` per call. A retry after a timeout therefore creates a **second live invoice** for the same booking. The withdrawal path does not exist yet, so it has the same defect waiting to be written.

## 2. Decision

Every money-moving attempt gets a **business-keyed idempotency key that is persisted before the provider call**, reused on every retry, and matched against a **request fingerprint** so a key can never be reused for a different amount or destination.

## 3. The key convention

| Path | Our key | Where it lives | Provider key |
|---|---|---|---|
| Collection | `SRB-<bookingID>-<paymentID>` | `payments.idempotency_key` (unique) | `Request-Id` header |
| Withdrawal | `PO-<bookingID>` | `payouts.partner_reference_no` (unique) | `partnerReferenceNo` body field |
| Withdrawal (secondary) | the payout row id, numeric | `payouts.id` | `X-EXTERNAL-ID` header |

The keys are **derived**, never random: a retry of the same persisted attempt reuses the same key by construction, and a genuinely new attempt is a new row with a new key. No key is ever generated at call time.

## 4. Replay handling

| Provider response | Meaning | Our behaviour |
|---|---|---|
| `2xx` | created | record the URL / reference numbers |
| `409` + a body carrying the original result | the same request already ran | **treat as success**, use the body, mark the attempt `replayed` |
| `409` + anything else | a genuine conflict | error |
| `4xx`/`5xx` otherwise | failed | error, the attempt stays retryable |

For the transfer API, a `409 Duplicate partnerReferenceNo` means *we already submitted this transfer* — not that it failed. The usecase reconciles it with `transactions-status` instead of retrying blindly.

## 5. Request fingerprint

Each idempotent attempt stores a fingerprint: `HMAC-SHA256(key, canonical-request)` (the same keyed-digest construction the payout feature already uses for account fingerprints). A retry whose fingerprint differs from the stored one is rejected with `IDEMPOTENCY_KEY_REUSED` — a key may never be reused for a different amount, destination, or booking.

## 6. Collection flow

1. `POST /api/v1/bookings` creates the booking and its payment row **before** any provider call. The payment row's `idempotency_key` and `request_fingerprint` are written at insert time.
2. The checkout call sends `Request-Id: <payments.idempotency_key>` and the same body as the first attempt. A `409` is read as a replay (§4).
3. **A failed attempt is retryable**: `POST /api/v1/bookings/{id}/payment` reuses the same payment row and therefore the same key and fingerprint. DOKU answers with the original checkout if it was created, or creates it if the first call never landed. Either way there is **one invoice**.
4. **Double-clicking "Bayar" cannot create two bookings**: `POST /api/v1/bookings` accepts an `Idempotency-Key` header. The key is stored on the booking row (unique index). A repeat with the same key and the same fingerprint returns the **existing booking** (and its existing payment and checkout URL) instead of creating a second one. A repeat with the same key and a different fingerprint is rejected with `IDEMPOTENCY_KEY_REUSED`.
5. The frontend generates one `Idempotency-Key` per booking submission (a UUID created when the submit starts, reused on retry), so a network retry or an impatient double-click is harmless.

## 7. Withdrawal flow (new)

A new `payouts` table — the ledger of money leaving the platform:

| Column | Notes |
|---|---|
| `id` | also the numeric `X-EXTERNAL-ID` |
| `booking_id` | **unique** — at most one payout per booking |
| `provider_id`, `amount`, `currency` | who and how much |
| `channel` | `BANK_ACCOUNT` (today) |
| `beneficiary_bank_code`, `beneficiary_account_holder_name_encrypted`, `beneficiary_account_number_encrypted`, `beneficiary_last4` | the destination; the account number is **encrypted** with the same `PAYOUT_ENCRYPTION_KEY` |
| `beneficiary_fingerprint` | keyed digest of the destination, so a changed destination is visible |
| `partner_reference_no` | **unique** — our `partnerReferenceNo`, the provider's idempotency key |
| `idempotency_key`, `request_fingerprint` | as for collection |
| `doku_inquiry_reference_no`, `doku_reference_no`, `doku_reference_number` | the provider's references, for reconciliation |
| `status` | `pending` → `inquiry` → `submitted` → `paid`, or `failed` |
| `replayed` | true when the provider answered 409 (we had already sent it) |
| `last_error`, `paid_at`, timestamps | |

The flow, with the guarantees:

1. `Release(ctx, bookingID)` loads the booking (must be `completed`, with a `paid` payment) and the provider's stored payout destination.
2. It creates the payout row — **one row per booking** (unique index). A second call finds the existing row and continues it instead of creating another.
3. `transfer-inquiry` with `partnerReferenceNo = <the stored key>` and the same body. A `409` means the inquiry already ran: reconcile with `transactions-status`.
4. `transfer-payment` with the inquiry's `referenceNo` and the same `partnerReferenceNo`. A `409` means **the transfer was already submitted**: mark the payout `submitted` and reconcile — it must never be retried as a new transfer.
5. The transfer webhook moves the payout to `paid` (idempotently: a repeated webhook does not change a `paid` payout).

**Amount:** the Mitra is paid the service amount (`bookings.sub_total`, which includes the add-ons). The platform fee stays with serabutin. The DOKU transfer fee is **not** deducted from the Mitra's amount yet: DOKU does not publish Sub-Account pricing, so the agreed policy ("the Mitra absorbs the DOKU fees") cannot be computed faithfully. This is recorded as an open decision in the handoff, not silently approximated.

**Trigger:** manual. An admin endpoint releases a booking's payout. The automatic trigger (work confirmed **and** the funds settled **and** the release window elapsed) is a later task, because settlement must be read from DOKU's balance API first.

## 8. Auth for the withdrawal API

The Sub-Account V2 API uses a different scheme from Checkout:

- `Authorization: Bearer <B2B token>` from `POST /adv-core-api/partner/v1.0/token` (body `{"grant_type":"client_credentials","valid_time":"360"}`, signed with the **non-SNAP** HMAC-SHA256 headers). The token is cached until shortly before it expires.
- `X-PARTNER-ID` (our Client ID), `X-TIMESTAMP` (ISO 8601 with timezone offset), `X-SIGNATURE` (base64 HMAC-SHA512), `X-EXTERNAL-ID` (numeric).

`stringToSign = HTTPMethod + ":" + EndpointURL + ":" + AccessToken + ":" + lowercase(hex(SHA256(minify(body)))) + ":" + Timestamp`

## 9. What this does not cover

- Refunds (the DOKU refund product) — the `debit/cancel` surface has its own `partnerReferenceNo` and gets the same treatment when it is built.
- The automatic release trigger and the settlement gate.
- Split Settlement (DOKU routes the money at settlement instead of us holding it) — a different model, not an idempotency problem.

## 10. Security checklist

- [x] No idempotency key is generated at call time — every key is derived from a persisted row
- [x] A key is unique per row and per attempt; a retry reuses it
- [x] A key reused with a different request is rejected (`IDEMPOTENCY_KEY_REUSED`)
- [x] A provider 409 carrying the original result is read as a replay, not a failure
- [x] The beneficiary account number is encrypted at rest like every other account number
- [x] Idempotency keys and fingerprints contain no account number or secret material

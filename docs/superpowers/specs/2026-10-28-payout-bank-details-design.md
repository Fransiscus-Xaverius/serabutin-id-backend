# Payout bank details (Mitra) — design

> Date: 2026-10-28 · Status: approved for implementation
> Related: [`spec/016.payments-prepay-doku.md`](../../../../serabutin-technical-documentation/spec/016.payments-prepay-doku.md), `research/2026-10-18-doku-payment-gateway-assessment.md` (rev. 4, §3.2 Design 1)

## 1. Context

The prepayment model collects customer money up front and releases it to the Mitra later. The founder confirmed that **Mitra do not get their own DOKU sub-accounts**: the release goes from serabutin's hub sub-account straight to the **Mitra's bank account** via DOKU's `transfer-payment` with `type: BANK_ACCOUNT`.

That means the platform must collect, store and protect the Mitra's bank destination before any payout can be made. Today the platform stores **no** bank details at all.

## 2. Decision

Add a `payout_accounts` table holding one payout destination per Mitra, with the sensitive fields **encrypted at rest** using AES-256-GCM. The encryption key lives only in the environment (`PAYOUT_ENCRYPTION_KEY`); the server refuses to start without it.

## 3. Data model

`payout_accounts` (one row per user, unique on `user_id`):

| Column | Type | Encrypted | Purpose |
|---|---|---|---|
| `user_id` | uint, unique | no | owner (a Mitra) |
| `bank_code` | varchar(16) | no | SWIFT/BIC code, e.g. `BCAIDJA` — public, needed for queries |
| `bank_name` | varchar(100) | no | display name, e.g. "Bank Central Asia" |
| `account_holder_name_encrypted` | text | **yes** | PII, sent to DOKU as `beneficiaryAccountName` |
| `account_number_encrypted` | text | **yes** | the account number |
| `account_last4` | varchar(4) | no | last four digits, for masked display |
| `account_fingerprint` | varchar(64), unique index | no | keyed HMAC of the account number — duplicate detection without decrypting |
| `encryption_key_id` | varchar(16) | no | which key encrypted the row (rotation) |
| `status` | varchar(20) | no | `unverified` → `verified` / `failed` |
| `name_mismatch` | bool | no | the holder name differs from the registered account name |
| `validated_at` | timestamptz | no | when DOKU's account validation confirmed it |
| `last_error` | varchar(255) | no | last validation failure |
| `created_at`, `updated_at` | timestamptz | no | |

Deliberately **not** stored: any decrypted value, any log line containing the account number, or any API response containing it.

### 3.1 Why this shape

- **Separate table, not more columns on `users`.** The users table is already wide; a separate table keeps the encryption concern, the validation status and the audit trail together, and leaves room for more than one destination later.
- **`last4` + `fingerprint` are the two things you can store in plaintext.** `last4` lets the UI show "•••• 1234" without decrypting; `fingerprint` detects two Mitras sharing one account without decrypting either.
- **`encryption_key_id` per row** is what makes rotation possible: re-encrypt under a new key, keep the old key to read rows written before the rotation.

## 4. Encryption

AES-256-GCM, implemented in `internal/infrastructure/encryption`:

- Ciphertext = `base64(nonce(12 bytes) || ciphertext || tag)`, nonce random per encryption.
- The key is `PAYOUT_ENCRYPTION_KEY`, base64-encoded, **exactly 32 bytes**.
- `PAYOUT_ENCRYPTION_KEY_ID` (default `v1`) is recorded on every row.
- The fingerprint is `HMAC-SHA256(key, "payout-account:v1:" + accountNumber)`, hex, truncated to 32 characters — a keyed digest, so it does not leak the number even if the row is dumped.

### 4.1 Key handling

- **Required at boot.** A missing, malformed or wrong-length key is a fatal startup error with the exact command to generate one. There is no fallback and no plaintext mode.
- The key lives in the git-ignored `.env` for local work and in the deployment secret store. It is never committed, never logged, never returned by an API.
- `docker-compose.yml` requires it explicitly (`${PAYOUT_ENCRYPTION_KEY:?...}`) so a container start cannot silently run without it.
- Rotation: add `PAYOUT_ENCRYPTION_KEY_ID=v2` with a new key, re-encrypt rows, drop the old key.

## 5. API

Both endpoints require a signed-in user with the `provider` role.

| Method | Path | Purpose |
|---|---|---|
| `PUT` | `/api/v1/payout-account` | Create or replace the caller's payout destination |
| `GET` | `/api/v1/payout-account` | Read the caller's payout destination (masked) |

Request body:

```json
{ "bank_code": "BCAIDJA", "bank_name": "Bank Central Asia",
  "account_holder_name": "Siti Rahayu", "account_number": "1234567890" }
```

Response body (**never** contains the account number or the holder name):

```json
{ "bank_code": "BCAIDJA", "bank_name": "Bank Central Asia",
  "account_last4": "7890", "status": "unverified",
  "name_mismatch": false, "updated_at": "2026-10-28T10:00:00Z" }
```

Status codes: `200` upsert/read · `400` malformed body or missing field · `401` no session · `403` not a Mitra · `409` `PAYOUT_ACCOUNT_DUPLICATE` · `422` invalid value (bank code, account number, holder name or bank name).

## 6. Validation

- `bank_code`: 4 to 11 characters, uppercase letters and digits (SWIFT/BIC) — pattern `^[A-Z0-9]{4,11}$`. This corrects the earlier rule of 4 or 8 characters (`^[A-Z0-9]{4}([A-Z0-9]{4})?$`), which would have rejected real Indonesian SWIFT/BIC codes of 7 characters — BCA's own BIC is `BCAIDJA` (7) — as well as any other code that is neither 4 nor 8 characters long. **DOKU's supported-bank list is the authority** on which bank codes are payable; this local rule only catches typos (lowercase letters, spaces, hyphens, wrong length).
- `account_number`: digits only, 6–32 characters (Indonesian bank accounts are numeric).
- `account_holder_name`: 2–100 characters, counted in runes after trimming (a multi-byte name is not rejected by its byte length).
- **Name match is flagged, not blocked.** If the normalised holder name differs from the registered account name, the row is stored with `name_mismatch = true` and status `unverified`. We cannot hard-block it because a Mitra may hold an account in a legal entity's name and the production entity (PT/CV) is still unresolved. The real gate is DOKU's account validation plus the customer-side confirmation of the beneficiary name at payout time.

## 7. Decryption boundary

Plaintext exists in exactly one place: inside `PayoutUsecase.DecryptDestination`, which returns the destination for the future DOKU transfer call. Nothing else in the codebase may call `Decrypt`, and no handler may expose it. The interface the HTTP handler receives, `usecase.PayoutAccountService`, exposes only `Upsert` and `Get`; `DecryptDestination` stays on the concrete `*PayoutUsecase`, so the type boundary enforces this.

## 7.1 Deployment

The fingerprint index is unique on a fresh database. AutoMigrate never changes an existing index, so a database created by the earlier non-unique model is upgraded on boot: `EnsurePayoutFingerprintIndexUnique` rebuilds the index as unique, and does nothing when it is already unique. If duplicate fingerprints exist, the server refuses to start and names the user ids to resolve. Nothing is dropped while duplicates exist.

## 8. Out of scope for this change

- DOKU's `Bank Account Validation` call and the `verified` status transition (needs the Sub-Account product).
- The `transfer-inquiry` / `transfer-payment` release path itself.
- Payout to e-wallets (DOKU cannot pay OVO/DANA/ShopeePay from a sub-account).
- Multiple payout destinations per Mitra.

## 9. Security checklist

- [x] Key required, fail-closed, never committed
- [x] AES-256-GCM with a per-value nonce
- [x] Account number and holder name encrypted at rest
- [x] No API response contains the account number or holder name
- [x] No log line contains the account number
- [x] Fingerprint for duplicate detection without decryption
- [x] Key id recorded per row for rotation
- [ ] Admin console cannot read the destination (enforced by there being no endpoint for it; the admin can only see `status`)

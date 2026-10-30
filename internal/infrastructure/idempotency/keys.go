// Package idempotency derives the idempotency keys and request fingerprints
// of the money-moving calls.
//
// Two rules hold everywhere in this package:
//
//   - A key is never generated at call time. It is derived from a persisted
//     row (the payment or the payout), so a retry of that row reuses the key
//     by construction and a genuinely new attempt gets a new key.
//   - A key never carries secret material: it holds row ids only.
//   - A fingerprint is a reuse detector, not a confidentiality control. It
//     is a plain SHA-256 of the canonical request, stored next to the key,
//     so the fields it covers can be guessed from the row. Never pass secret
//     material (a bank account number, a token) to Fingerprint; secret
//     material uses the encryption-keyed digest instead (see
//     encryption.Cipher.Fingerprint).
package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// CollectionKey is the idempotency key of a checkout attempt. It is derived
// from the persisted payment row, so a retry of that attempt reuses it and a
// genuinely new attempt (a new payment row) gets a new key.
func CollectionKey(bookingID, paymentID uint) string {
	return fmt.Sprintf("SRB-%d-%d", bookingID, paymentID)
}

// PayoutKey is the idempotency key of the withdrawal of one booking. At most
// one payout row exists per booking, so the key is unique by construction.
func PayoutKey(bookingID uint) string {
	return fmt.Sprintf("PO-%d", bookingID)
}

// CanonicalRequest joins the parts of a request in a fixed order with "|".
// The collection and the payout path both build their canonical string
// through it, so the two paths cannot drift apart.
func CanonicalRequest(parts ...string) string {
	return strings.Join(parts, "|")
}

// CanonicalPaymentRequest builds the canonical request string of a checkout
// attempt: invoice|amount|currency|channel|expiryMinutes. Those are the
// fields that define the charge, so a retry that changes any of them
// produces a different fingerprint and is refused instead of silently
// charged a second time.
func CanonicalPaymentRequest(invoice string, amount int64, currency, channel string, expiryMinutes int) string {
	return CanonicalRequest(
		invoice,
		strconv.FormatInt(amount, 10),
		currency,
		channel,
		strconv.Itoa(expiryMinutes),
	)
}

// Fingerprint identifies the request a key was used for: a retry whose
// fingerprint differs from the stored one is a different request under the
// same key. It is the plain SHA-256 hex digest of the canonical request,
// truncated to 32 characters. The fingerprint is stored next to its key, so
// it does not hide the request from anyone who can read the row.
//
// EXCEPTION: a fingerprint over secret material (for example the beneficiary
// bank account number of the payout path) must NOT use this helper. It uses
// the encryption-keyed digest, encryption.Cipher.Fingerprint, which needs the
// server's key to compute.
func Fingerprint(canonicalRequest string) string {
	sum := sha256.Sum256([]byte(canonicalRequest))
	return hex.EncodeToString(sum[:])[:32]
}

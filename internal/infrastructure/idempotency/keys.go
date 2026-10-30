// Package idempotency derives the idempotency keys and request fingerprints
// of the money-moving calls.
//
// Two rules hold everywhere in this package:
//
//   - A key is never generated at call time. It is derived from a persisted
//     row (the payment or the payout), so a retry of that row reuses the key
//     by construction and a genuinely new attempt gets a new key.
//   - A key and a fingerprint never carry secret material. A key holds row
//     ids only; a fingerprint is a keyed digest, so it cannot be read back
//     into the request it covers.
package idempotency

import (
	"crypto/hmac"
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

// Fingerprint identifies the request a key was used for. It is a keyed
// digest, so it reveals nothing about the request and cannot be brute-forced
// from the row.
func Fingerprint(key, canonicalRequest string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(canonicalRequest))
	return hex.EncodeToString(mac.Sum(nil))[:32]
}

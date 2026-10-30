package idempotency

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollectionKey_IsStableAndUniquePerPayment(t *testing.T) {
	assert.Equal(t, "SRB-42-7", CollectionKey(42, 7))
	assert.Equal(t, "SRB-42-7", CollectionKey(42, 7))
	assert.NotEqual(t, CollectionKey(42, 7), CollectionKey(42, 8))
	assert.NotEqual(t, CollectionKey(42, 7), CollectionKey(43, 7))
}

func TestPayoutKey_IsStablePerBooking(t *testing.T) {
	assert.Equal(t, "PO-42", PayoutKey(42))
	assert.NotEqual(t, PayoutKey(42), PayoutKey(43))
}

func TestFingerprint_IsStableForTheSameRequestAndDiffersOtherwise(t *testing.T) {
	first := Fingerprint("SRB-42-7", `{"amount":155000,"channel":"qris"}`)
	assert.Equal(t, first, Fingerprint("SRB-42-7", `{"amount":155000,"channel":"qris"}`))
	assert.NotEqual(t, first, Fingerprint("SRB-42-7", `{"amount":155001,"channel":"qris"}`))
	assert.Len(t, first, 32)
	assert.NotContains(t, first, "155000")
}

func TestCollectionKeyAndPayoutKey_CarryNoSecretMaterial(t *testing.T) {
	// The keys are derived from row ids only: they must never carry an
	// account number, a token or any other secret.
	assert.Equal(t, []string{"SRB", "42", "7"}, strings.Split(CollectionKey(42, 7), "-"))
	assert.Equal(t, []string{"PO", "42"}, strings.Split(PayoutKey(42), "-"))
}

func TestCanonicalPaymentRequest_IsTheOrderedFieldList(t *testing.T) {
	assert.Equal(t, "SRB-42-7-1|155000|IDR|qris|30", CanonicalPaymentRequest("SRB-42-7-1", 155000, "IDR", "qris", 30))
}

func TestCanonicalPaymentRequest_ChangesWhenTheChargeChanges(t *testing.T) {
	base := CanonicalPaymentRequest("SRB-42-7-1", 155000, "IDR", "qris", 30)

	assert.NotEqual(t, base, CanonicalPaymentRequest("SRB-42-7-2", 155000, "IDR", "qris", 30), "the invoice number is part of the charge")
	assert.NotEqual(t, base, CanonicalPaymentRequest("SRB-42-7-1", 155001, "IDR", "qris", 30), "the amount is part of the charge")
	assert.NotEqual(t, base, CanonicalPaymentRequest("SRB-42-7-1", 155000, "USD", "qris", 30), "the currency is part of the charge")
	assert.NotEqual(t, base, CanonicalPaymentRequest("SRB-42-7-1", 155000, "IDR", "dana", 30), "the channel is part of the charge")
	assert.NotEqual(t, base, CanonicalPaymentRequest("SRB-42-7-1", 155000, "IDR", "qris", 60), "the expiry is part of the charge")
}

func TestCanonicalRequest_JoinsThePartsInAFixedOrder(t *testing.T) {
	assert.Equal(t, "a|b|c", CanonicalRequest("a", "b", "c"))
	assert.Equal(t, "a|b|c", CanonicalRequest("a", "b", "c"))
	assert.NotEqual(t, CanonicalRequest("a", "b", "c"), CanonicalRequest("c", "b", "a"))
}

func TestFingerprint_DiffersForADifferentKeyOnTheSameRequest(t *testing.T) {
	request := CanonicalPaymentRequest("SRB-42-7-1", 155000, "IDR", "qris", 30)

	assert.NotEqual(t, Fingerprint("SRB-42-7", request), Fingerprint("SRB-42-8", request))
}

func TestFingerprint_IsHexAndHidesTheRequest(t *testing.T) {
	request := CanonicalPaymentRequest("SRB-42-7-1", 155000, "IDR", "qris", 30)
	fingerprint := Fingerprint("SRB-42-7", request)

	assert.Len(t, fingerprint, 32)
	assert.NotContains(t, fingerprint, "155000")
	assert.NotContains(t, fingerprint, "qris")
	assert.Regexp(t, "^[0-9a-f]{32}$", fingerprint)
}

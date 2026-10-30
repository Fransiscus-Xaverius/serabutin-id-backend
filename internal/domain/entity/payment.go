package entity

import "time"

const (
	PaymentStatusPending       = "pending"
	PaymentStatusFailed        = "failed"
	PaymentStatusPaid          = "paid"
	PaymentStatusExpired       = "expired"
	PaymentStatusRefundPending = "refund_pending"
	PaymentStatusRefunded      = "refunded"

	PaymentChannelQRIS = "qris"

	PaymentProviderDOKU = "doku"
	PaymentProviderMock = "mock"
)

// PaymentChannelDOKUNames maps the channel slug we use in the API and the
// UI to the channel name DOKU Checkout expects in
// payment.payment_method_types. The slug is what a booking stores; the DOKU
// name only exists at the provider boundary, so opening a new channel is a
// configuration change rather than a code change.
var PaymentChannelDOKUNames = map[string]string{
	"qris":              "QRIS",
	"dana":              "EMONEY_DANA",
	"ovo":               "EMONEY_OVO",
	"shopee":            "EMONEY_SHOPEE_PAY",
	"linkaja":           "EMONEY_LINKAJA",
	"card":              "CREDIT_CARD",
	"direct_debit":      "DIRECT_DEBIT_BRI",
	"va_bca":            "VIRTUAL_ACCOUNT_BCA",
	"va_mandiri":        "VIRTUAL_ACCOUNT_BANK_MANDIRI",
	"va_bsi":            "VIRTUAL_ACCOUNT_BANK_SYARIAH_MANDIRI",
	"va_doku":           "VIRTUAL_ACCOUNT_DOKU",
	"va_bri":            "VIRTUAL_ACCOUNT_BRI",
	"va_bni":            "VIRTUAL_ACCOUNT_BNI",
	"va_permata":        "VIRTUAL_ACCOUNT_BANK_PERMATA",
	"va_cimb":           "VIRTUAL_ACCOUNT_BANK_CIMB",
	"va_danamon":        "VIRTUAL_ACCOUNT_BANK_DANAMON",
	"cstore_alfa":       "ONLINE_TO_OFFLINE_ALFA",
	"cstore_indomaret":  "ONLINE_TO_OFFLINE_INDOMARET",
	"paylater_akulaku":  "PEER_TO_PEER_AKULAKU",
	"paylater_kredivo":  "PEER_TO_PEER_KREDIVO",
	"paylater_indodana": "PEER_TO_PEER_INDODANA",
}

// DOKUChannelName returns the DOKU channel name for a slug. Unknown slugs
// are rejected rather than silently sent to the provider.
func DOKUChannelName(slug string) (string, bool) {
	name, ok := PaymentChannelDOKUNames[slug]
	return name, ok
}

type Payment struct {
	ID                uint
	BookingID         uint
	InvoiceNumber     string
	Amount            int64
	Currency          string
	Channel           string
	Status            string
	Provider          string
	ProviderRequestID string
	ProviderSessionID string
	PaymentURL        string
	ExpiresAt         *time.Time
	PaidAt            *time.Time
	LastError         string
	// IdempotencyKey is the key this attempt is sent to the provider with.
	// It is derived from the persisted row, so a retry reuses it. It is
	// empty while the attempt has not been keyed yet.
	IdempotencyKey string
	// RequestFingerprint identifies the request the key was used for. It is
	// a SHA-256 digest of the canonical request, a reuse detector and not a
	// confidentiality control; the canonical request holds no secret.
	RequestFingerprint string
	// Replayed reports that the provider answered this attempt with the
	// original result of an earlier one instead of creating a new one.
	Replayed  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

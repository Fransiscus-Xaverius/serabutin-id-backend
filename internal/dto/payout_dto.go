package dto

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	// DOKU Kirim DOKU SNAP uses the numeric Bank Indonesia bank code
	// (for example 014 for BCA), not a SWIFT/BIC.
	bankCodePattern      = regexp.MustCompile(`^[0-9]{2,8}$`)
	accountNumberPattern = regexp.MustCompile(`^[0-9]{6,32}$`)
	phonePattern         = regexp.MustCompile(`^[0-9]{8,16}$`)
)

type PayoutAccountRequest struct {
	BankCode          string `json:"bank_code" validate:"required"`
	BankName          string `json:"bank_name" validate:"required"`
	AccountHolderName string `json:"account_holder_name" validate:"required"`
	AccountNumber     string `json:"account_number" validate:"required"`
	Phone             string `json:"phone" validate:"required"`
}

// Validate catches typos in the submitted fields: a bank code that is not a
// numeric BI code, an account number that is not 6 to 32 digits, or a phone
// that is not 8 to 16 digits. DOKU's supported-bank list is the authority on
// which bank codes are payable; this rule only rejects malformed input.
func (r PayoutAccountRequest) Validate() error {
	if !bankCodePattern.MatchString(r.BankCode) {
		return errors.New("bank_code must be a 2 to 8 digit BI bank code")
	}
	if !accountNumberPattern.MatchString(r.AccountNumber) {
		return errors.New("account_number must be 6 to 32 digits")
	}
	if !phonePattern.MatchString(r.Phone) {
		return errors.New("phone must be 8 to 16 digits")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(r.AccountHolderName)); n < 2 || n > 64 {
		return errors.New("account_holder_name must be 2 to 64 characters")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(r.BankName)); n < 2 || n > 100 {
		return errors.New("bank_name must be 2 to 100 characters")
	}
	return nil
}

type PayoutAccountResponse struct {
	BankCode     string `json:"bank_code"`
	BankName     string `json:"bank_name"`
	AccountLast4 string `json:"account_last4"`
	PhoneLast4   string `json:"phone_last4"`
	Status       string `json:"status"`
	NameMismatch bool   `json:"name_mismatch"`
	UpdatedAt    string `json:"updated_at"`
}

// PayoutReleaseResponse is the payout of a booking as the admin sees it. It
// carries the last four digits of the destination, never the full number.
type PayoutReleaseResponse struct {
	ID                 uint   `json:"id"`
	BookingID          uint   `json:"booking_id"`
	Status             string `json:"status"`
	Amount             int64  `json:"amount"`
	Currency           string `json:"currency"`
	PartnerReferenceNo string `json:"partner_reference_no"`
	BankCode           string `json:"bank_code"`
	AccountLast4       string `json:"account_last4"`
	Replayed           bool   `json:"replayed"`
}

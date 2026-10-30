package dto

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	bankCodePattern      = regexp.MustCompile(`^[A-Z0-9]{4,11}$`)
	accountNumberPattern = regexp.MustCompile(`^[0-9]{6,32}$`)
)

type PayoutAccountRequest struct {
	BankCode          string `json:"bank_code" validate:"required"`
	BankName          string `json:"bank_name" validate:"required"`
	AccountHolderName string `json:"account_holder_name" validate:"required"`
	AccountNumber     string `json:"account_number" validate:"required"`
}

// Validate catches typos in the submitted fields: a bank code with lowercase
// letters, spaces, hyphens or the wrong length, and an account number that is
// not 6 to 32 digits. DOKU's supported-bank list is the authority on which bank
// codes are payable; this rule only rejects malformed input.
func (r PayoutAccountRequest) Validate() error {
	if !bankCodePattern.MatchString(r.BankCode) {
		return errors.New("bank_code must be 4 to 11 uppercase letters or digits")
	}
	if !accountNumberPattern.MatchString(r.AccountNumber) {
		return errors.New("account_number must be 6 to 32 digits")
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(r.AccountHolderName)); n < 2 || n > 100 {
		return errors.New("account_holder_name must be 2 to 100 characters")
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

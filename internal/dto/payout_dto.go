package dto

import (
	"errors"
	"regexp"
	"strings"
)

var (
	bankCodePattern      = regexp.MustCompile(`^[A-Z0-9]{4}([A-Z0-9]{4})?$`)
	accountNumberPattern = regexp.MustCompile(`^[0-9]{6,32}$`)
)

type PayoutAccountRequest struct {
	BankCode          string `json:"bank_code" validate:"required"`
	BankName          string `json:"bank_name" validate:"required"`
	AccountHolderName string `json:"account_holder_name" validate:"required"`
	AccountNumber     string `json:"account_number" validate:"required"`
}

// Validate enforces the shapes DOKU expects: a SWIFT/BIC bank code and a
// numeric account number.
func (r PayoutAccountRequest) Validate() error {
	if !bankCodePattern.MatchString(r.BankCode) {
		return errors.New("bank_code must be 4 or 8 uppercase letters or digits")
	}
	if !accountNumberPattern.MatchString(r.AccountNumber) {
		return errors.New("account_number must be 6 to 32 digits")
	}
	if len(strings.TrimSpace(r.AccountHolderName)) < 2 {
		return errors.New("account_holder_name is required")
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

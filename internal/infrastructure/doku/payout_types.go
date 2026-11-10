package doku

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// SNAP transaction status codes from check-status (latestTransactionStatus).
const (
	TransactionStatusSuccess  = "00"
	TransactionStatusPending  = "03"
	TransactionStatusRefunded = "04"
	TransactionStatusFailed   = "06"
)

// Balance account types. Only DOKU_MERCHANT_IDR is settled and withdrawable.
const (
	BalanceAccountMerchantIDR        = "DOKU_MERCHANT_IDR"
	BalanceAccountMerchantPendingIDR = "DOKU_MERCHANT_PENDING_IDR"
	BalanceAccountMerchantPoint      = "DOKU_MERCHANT_POINT"
)

// ErrTransactionNotFound means DOKU has no record of the partner reference.
// Only this error proves that no transfer exists.
var ErrTransactionNotFound = errors.New("doku has no record of this partner reference")

var numericPattern = regexp.MustCompile(`^[0-9]+$`)

func isNumeric(value string) bool {
	return numericPattern.MatchString(value)
}

// SignSHA512 returns base64(HMAC-SHA512(secret, stringToSign)) where
// stringToSign is method:path:token:lowercase(hex(SHA256(minify(body)))):timestamp.
func SignSHA512(clientSecret, method, endpointURL, accessToken, body, timestamp string) string {
	minified := body
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(body)); err == nil {
		minified = compact.String()
	}
	sum := sha256.Sum256([]byte(minified))
	bodyDigest := hex.EncodeToString(sum[:])

	stringToSign := strings.Join([]string{method, endpointURL, accessToken, bodyDigest, timestamp}, ":")
	mac := hmac.New(sha512.New, []byte(clientSecret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// TransferInquiryRequest is the SNAP account-inquiry input. FromAccount is
// unused by SNAP; it remains only because callers share the struct shape.
type TransferInquiryRequest struct {
	PartnerReferenceNo       string
	Amount                   int64
	FromAccount              string
	BeneficiaryBankCode      string
	BeneficiaryAccountNumber string
	Remark                   string

	CustomerNumber      string
	BeneficiaryName     string
	BeneficiaryPhone    string
	BeneficiaryBankName string
	SenderCountryCode   string
}

type TransferInquiryResult struct {
	ReferenceNo            string
	PartnerReferenceNo     string
	BeneficiaryAccountName string
	SessionID              string
	Replayed               bool
}

type TransferPaymentRequest struct {
	TransferInquiryRequest
	ReferenceNo            string
	SessionID              string
	BeneficiaryAccountName string
}

type TransferPaymentResult struct {
	ReferenceNo        string
	PartnerReferenceNo string
	ReferenceNumber    string
	TransactionDate    string
	Replayed           bool
}

// Money is a DOKU amount: a decimal string and its currency.
type Money struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type TransactionStatusRequest struct {
	PartnerReferenceNo       string
	BeneficiaryAccountNumber string
	BeneficiaryBankCode      string
}

type TransactionStatusResult struct {
	PartnerReferenceNo  string
	OriginalReferenceNo string
	SessionID           string
	TransactionType     string
	LatestStatus        string
	LatestStatusDesc    string
	TransactionDate     string
	Amount              Money
	RefundHistory       []TransactionRefund
}

type TransactionRefund struct {
	RefundNo        string
	RefundStatus    string
	TransactionDate string
	Reason          string
	RefundAmount    Money
}

type BalanceResult struct {
	ProfileID string
	Name      string
	Accounts  []BalanceAccount
}

type BalanceAccount struct {
	Type      string
	Currency  string
	AccountNo string
	Available string
	Reserved  string
}

// SettledAvailable returns the withdrawable balance: the available amount of
// the DOKU_MERCHANT_IDR account.
func (r *BalanceResult) SettledAvailable() (string, bool) {
	for _, account := range r.Accounts {
		if account.Type == BalanceAccountMerchantIDR {
			return account.Available, true
		}
	}
	return "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

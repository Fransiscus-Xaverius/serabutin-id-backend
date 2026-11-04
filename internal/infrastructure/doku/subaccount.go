package doku

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	subAccountTokenPath          = "/adv-core-api/partner/v1.0/token"
	subAccountInquiryPath        = "/sub-account/v2.0/transfer-inquiry"
	subAccountPaymentPath        = "/sub-account/v2.0/transfer-payment"
	subAccountStatusPath         = "/sub-account/v2.0/transactions-status"
	subAccountBalancePath        = "/sub-account/v2.0/balance-inquiries"
	subAccountTokenValidSeconds  = 360
	subAccountTokenRefreshMargin = 60 * time.Second
	subAccountTimestampLayout    = "2006-01-02T15:04:05-07:00"
	subAccountTransferType       = "BANK_ACCOUNT"
	subAccountChannel            = "BI_FAST"
	subAccountCurrency           = "IDR"
	subAccountMaxResponseBytes   = 1 << 20
)

var (
	subAccountNumeric      = regexp.MustCompile(`^[0-9]+$`)
	subAccountResponseCode = regexp.MustCompile(`^[0-9]{7}$`)
)

// Transaction status codes from transactions-status (latestTransactionStatus).
const (
	TransactionStatusSuccess  = "00"
	TransactionStatusPending  = "03"
	TransactionStatusRefunded = "04"
	TransactionStatusCanceled = "05"
	TransactionStatusFailed   = "06"
)

// Balance account types from balance-inquiries. Only DOKU_MERCHANT_IDR is
// settled and withdrawable; DOKU_MERCHANT_PENDING_IDR is held.
const (
	BalanceAccountMerchantIDR        = "DOKU_MERCHANT_IDR"
	BalanceAccountMerchantPendingIDR = "DOKU_MERCHANT_PENDING_IDR"
	BalanceAccountMerchantPoint      = "DOKU_MERCHANT_POINT"
)

var subAccountZone = time.FixedZone("WIB", 7*60*60)

// SubAccountClient pays a Mitra's bank account through DOKU Sub-Account V2.
// It authenticates with a cached B2B bearer token and signs each call with
// HMAC-SHA512 over the request body. Config is the same as the Checkout
// client: ClientID is the partner id, SecretKey signs requests.
type SubAccountClient struct {
	cfg        Config
	httpClient *http.Client
	now        func() time.Time

	mu           sync.Mutex
	token        string
	tokenExpires time.Time
}

func NewSubAccountClient(cfg Config) *SubAccountClient {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultRequestTimeout}
	}

	return &SubAccountClient{cfg: cfg, httpClient: httpClient, now: time.Now}
}

// TransferInquiry asks DOKU to validate a transfer and resolve the beneficiary
// account name. The name must be shown to the user for confirmation before
// TransferPayment. PartnerReferenceNo is the caller's idempotency key.
func (c *SubAccountClient) TransferInquiry(ctx context.Context, externalID string, req TransferInquiryRequest) (*TransferInquiryResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	if req.PartnerReferenceNo == "" {
		return nil, fmt.Errorf("doku transfer inquiry requires a partner reference no (the idempotency key)")
	}

	parsed, replayed, err := c.transfer(ctx, externalID, subAccountInquiryPath, req.body())
	if err != nil {
		return nil, err
	}

	return &TransferInquiryResult{
		ReferenceNo:            parsed.ReferenceNo,
		PartnerReferenceNo:     firstNonEmpty(parsed.PartnerReferenceNo, req.PartnerReferenceNo),
		BeneficiaryAccountName: parsed.BeneficiaryAccountName,
		Replayed:               replayed,
	}, nil
}

// TransferPayment executes a transfer that TransferInquiry validated.
func (c *SubAccountClient) TransferPayment(ctx context.Context, externalID string, req TransferPaymentRequest) (*TransferPaymentResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	if req.PartnerReferenceNo == "" {
		return nil, fmt.Errorf("doku transfer payment requires a partner reference no (the idempotency key)")
	}
	if req.ReferenceNo == "" {
		return nil, fmt.Errorf("doku transfer payment requires the inquiry reference no")
	}

	body := transferPaymentBody{
		transferBody:           req.body(),
		ReferenceNo:            req.ReferenceNo,
		BeneficiaryAccountName: req.BeneficiaryAccountName,
	}
	parsed, replayed, err := c.transfer(ctx, externalID, subAccountPaymentPath, body)
	if err != nil {
		return nil, err
	}

	return &TransferPaymentResult{
		ReferenceNo:        parsed.ReferenceNo,
		PartnerReferenceNo: firstNonEmpty(parsed.PartnerReferenceNo, req.PartnerReferenceNo),
		ReferenceNumber:    parsed.ReferenceNumber,
		TransactionDate:    parsed.TransactionDate,
		Replayed:           replayed,
	}, nil
}

// TransactionStatus reads the latest status of a transfer by its partner
// reference no. Compare LatestStatus against the TransactionStatus constants.
func (c *SubAccountClient) TransactionStatus(ctx context.Context, externalID string, partnerReferenceNo string) (*TransactionStatusResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	if partnerReferenceNo == "" {
		return nil, fmt.Errorf("doku transaction status requires a partner reference no")
	}

	var parsed transactionStatusResponse
	err := c.postJSON(ctx, externalID, subAccountStatusPath, transactionStatusRequest{PartnerReferenceNo: partnerReferenceNo}, &parsed)
	if err != nil {
		return nil, err
	}
	if parsed.PartnerReferenceNo != partnerReferenceNo {
		return nil, fmt.Errorf("doku sub-account %s response does not match partner reference no %q", subAccountStatusPath, partnerReferenceNo)
	}
	if parsed.LatestTransactionStatus == "" {
		return nil, fmt.Errorf("doku sub-account %s response missing latestTransactionStatus", subAccountStatusPath)
	}

	refunds := make([]TransactionRefund, 0, len(parsed.RefundHistory))
	for _, refund := range parsed.RefundHistory {
		refunds = append(refunds, TransactionRefund{
			RefundNo:        refund.RefundNo,
			RefundStatus:    refund.RefundStatus,
			TransactionDate: refund.TransactionDate,
			Reason:          refund.Reason,
			RefundAmount:    refund.RefundAmount,
		})
	}

	return &TransactionStatusResult{
		PartnerReferenceNo: parsed.PartnerReferenceNo,
		TransactionType:    parsed.TransactionType,
		LatestStatus:       parsed.LatestTransactionStatus,
		LatestStatusDesc:   parsed.LatestTransactionDesc,
		TransactionDate:    parsed.TransactionDate,
		Amount:             parsed.Amount,
		RefundHistory:      refunds,
	}, nil
}

// BalanceInquiry reads the balances of a hub profile. Gate a release on the
// settled DOKU_MERCHANT_IDR available balance (see SettledAvailable).
func (c *SubAccountClient) BalanceInquiry(ctx context.Context, externalID string, profileID string) (*BalanceResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	if profileID == "" {
		return nil, fmt.Errorf("doku balance inquiry requires a profile id")
	}

	var parsed balanceResponse
	err := c.postJSON(ctx, externalID, subAccountBalancePath, balanceRequest{ProfileID: profileID}, &parsed)
	if err != nil {
		return nil, err
	}

	accounts := make([]BalanceAccount, 0, len(parsed.Accounts))
	for _, account := range parsed.Accounts {
		accounts = append(accounts, BalanceAccount{
			Type:      account.Type,
			Currency:  account.Currency,
			AccountNo: account.AccountNo,
			Available: account.Balance.Available,
			Reserved:  account.Balance.Reserved,
		})
	}

	return &BalanceResult{
		ProfileID: parsed.ProfileID,
		Name:      parsed.Name,
		Accounts:  accounts,
	}, nil
}

type TransferInquiryRequest struct {
	PartnerReferenceNo       string
	Amount                   int64
	FromAccount              string
	BeneficiaryBankCode      string
	BeneficiaryAccountNumber string
	Remark                   string
}

type TransferInquiryResult struct {
	ReferenceNo            string
	PartnerReferenceNo     string
	BeneficiaryAccountName string
	Replayed               bool
}

type TransferPaymentRequest struct {
	TransferInquiryRequest
	ReferenceNo            string
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

type TransactionStatusResult struct {
	PartnerReferenceNo string
	TransactionType    string
	LatestStatus       string
	LatestStatusDesc   string
	TransactionDate    string
	Amount             Money
	RefundHistory      []TransactionRefund
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
// the DOKU_MERCHANT_IDR account. The pending account is never withdrawable.
func (r *BalanceResult) SettledAvailable() (string, bool) {
	for _, account := range r.Accounts {
		if account.Type == BalanceAccountMerchantIDR {
			return account.Available, true
		}
	}
	return "", false
}

func (c *SubAccountClient) requireConfigured() error {
	if c.cfg.ClientID == "" || c.cfg.SecretKey == "" {
		return fmt.Errorf("doku sub-account is not configured: DOKU_CLIENT_ID and DOKU_SECRET_KEY are required")
	}
	return nil
}

// transfer posts a transfer call. A 409 that carries a reference is the
// original transfer answering a repeated partnerReferenceNo, so it is a replay.
// A 2xx without a referenceNo is an error: the transfer state is unknown.
func (c *SubAccountClient) transfer(ctx context.Context, externalID string, path string, payload any) (transferResponse, bool, error) {
	status, raw, err := c.call(ctx, externalID, path, payload)
	if err != nil {
		return transferResponse{}, false, err
	}

	var parsed transferResponse
	if status == http.StatusConflict {
		if err := json.Unmarshal(raw, &parsed); err == nil && parsed.ReferenceNo != "" {
			return parsed, true, nil
		}
	}
	if status < 200 || status >= 300 {
		return transferResponse{}, false, dokuError(path, status, raw)
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return transferResponse{}, false, fmt.Errorf("parse doku sub-account %s response", path)
	}
	if parsed.ReferenceNo == "" {
		return transferResponse{}, false, fmt.Errorf("doku sub-account %s response missing referenceNo", path)
	}

	return parsed, false, nil
}

func (c *SubAccountClient) postJSON(ctx context.Context, externalID string, path string, payload any, out any) error {
	status, raw, err := c.call(ctx, externalID, path, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return dokuError(path, status, raw)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parse doku sub-account %s response", path)
	}
	return nil
}

// call sends one signed Sub-Account request. externalID is the X-EXTERNAL-ID
// the caller owns (the payout row id); it is checked before any call to DOKU.
func (c *SubAccountClient) call(ctx context.Context, externalID string, path string, payload any) (int, []byte, error) {
	if !isNumeric(externalID) {
		return 0, nil, fmt.Errorf("doku sub-account %s requires a numeric external id (the payout row id)", path)
	}

	token, err := c.accessToken(ctx)
	if err != nil {
		return 0, nil, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal doku sub-account %s request: %w", path, err)
	}
	timestamp := c.now().In(subAccountZone).Format(subAccountTimestampLayout)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("build doku sub-account %s request: %w", path, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("X-PARTNER-ID", c.cfg.ClientID)
	httpReq.Header.Set("X-EXTERNAL-ID", externalID)
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", SignSHA512(c.cfg.SecretKey, http.MethodPost, path, token, string(body), timestamp))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, nil, fmt.Errorf("call doku sub-account %s: %w", path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, subAccountMaxResponseBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("read doku sub-account %s response: %w", path, err)
	}
	return resp.StatusCode, raw, nil
}

// accessToken returns the cached B2B token, fetching a new one only when the
// cache is empty or past its refresh point.
func (c *SubAccountClient) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && c.now().Before(c.tokenExpires) {
		return c.token, nil
	}

	token, err := c.fetchToken(ctx)
	if err != nil {
		return "", err
	}
	c.token = token
	c.tokenExpires = c.now().Add(subAccountTokenValidSeconds*time.Second - subAccountTokenRefreshMargin)
	return token, nil
}

func (c *SubAccountClient) fetchToken(ctx context.Context) (string, error) {
	body, err := json.Marshal(tokenRequest{GrantType: "client_credentials", ValidTime: strconv.Itoa(subAccountTokenValidSeconds)})
	if err != nil {
		return "", fmt.Errorf("marshal doku token request: %w", err)
	}

	requestID := uuid.NewString()
	timestamp := c.now().UTC().Format(timestampLayout)
	signature := Sign(c.cfg.ClientID, requestID, timestamp, subAccountTokenPath, Digest(body), c.cfg.SecretKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.BaseURL, "/")+subAccountTokenPath, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build doku token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Client-Id", c.cfg.ClientID)
	httpReq.Header.Set("Request-Id", requestID)
	httpReq.Header.Set("Request-Timestamp", timestamp)
	httpReq.Header.Set("Signature", signaturePrefix+signature)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call doku token: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, subAccountMaxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("read doku token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("doku token returned HTTP %d", resp.StatusCode)
	}

	var parsed tokenResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("parse doku token response")
	}
	if parsed.Token == "" {
		return "", fmt.Errorf("doku token response missing token")
	}
	return parsed.Token, nil
}

// dokuError describes a non-2xx answer. The response body is never included:
// it can echo the beneficiary account number. DOKU's responseCode is a machine
// code, so it is kept only when it has the documented seven-digit shape.
func dokuError(path string, status int, raw []byte) error {
	var parsed struct {
		ResponseCode string `json:"responseCode"`
	}
	if json.Unmarshal(raw, &parsed) == nil && subAccountResponseCode.MatchString(parsed.ResponseCode) {
		return fmt.Errorf("doku sub-account %s returned HTTP %d (responseCode %s)", path, status, parsed.ResponseCode)
	}
	return fmt.Errorf("doku sub-account %s returned HTTP %d", path, status)
}

func isNumeric(value string) bool {
	return subAccountNumeric.MatchString(value)
}

// SignSHA512 returns base64(HMAC-SHA512(secret, stringToSign)) where
// stringToSign is method:endpoint:token:lowercase(hex(SHA256(minify(body)))):timestamp.
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

func (req TransferInquiryRequest) body() transferBody {
	return transferBody{
		PartnerReferenceNo: req.PartnerReferenceNo,
		Type:               subAccountTransferType,
		Amount: Money{
			Value:    fmt.Sprintf("%d.00", req.Amount),
			Currency: subAccountCurrency,
		},
		Channel:                  subAccountChannel,
		FromAccount:              req.FromAccount,
		BeneficiaryBankCode:      req.BeneficiaryBankCode,
		BeneficiaryAccountNumber: req.BeneficiaryAccountNumber,
		Remark:                   req.Remark,
	}
}

type tokenRequest struct {
	GrantType string `json:"grant_type"`
	ValidTime string `json:"valid_time"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

type transferBody struct {
	PartnerReferenceNo       string `json:"partnerReferenceNo"`
	Type                     string `json:"type"`
	Amount                   Money  `json:"amount"`
	Channel                  string `json:"channel"`
	FromAccount              string `json:"fromAccount"`
	BeneficiaryBankCode      string `json:"beneficiaryBankCode"`
	BeneficiaryAccountNumber string `json:"beneficiaryAccountNumber"`
	Remark                   string `json:"remark"`
}

type transferPaymentBody struct {
	transferBody
	ReferenceNo            string `json:"referenceNo"`
	BeneficiaryAccountName string `json:"beneficiaryAccountName"`
}

// transferResponse covers the transfer-inquiry and transfer-payment responses
// and the 409 replay body. Each endpoint fills the fields it documents.
type transferResponse struct {
	ReferenceNo            string `json:"referenceNo"`
	PartnerReferenceNo     string `json:"partnerReferenceNo"`
	BeneficiaryAccountName string `json:"beneficiaryAccountName"`
	ReferenceNumber        string `json:"referenceNumber"`
	TransactionDate        string `json:"transactionDate"`
}

type transactionStatusRequest struct {
	PartnerReferenceNo string `json:"partnerReferenceNo"`
}

type transactionStatusResponse struct {
	PartnerReferenceNo      string              `json:"partnerReferenceNo"`
	TransactionType         string              `json:"transactionType"`
	LatestTransactionStatus string              `json:"latestTransactionStatus"`
	LatestTransactionDesc   string              `json:"latestTransactionDesc"`
	TransactionDate         string              `json:"transactionDate"`
	Amount                  Money               `json:"amount"`
	RefundHistory           []refundHistoryItem `json:"refundHistory"`
}

type refundHistoryItem struct {
	RefundNo        string `json:"refundNo"`
	RefundStatus    string `json:"refundStatus"`
	TransactionDate string `json:"transactionDate"`
	Reason          string `json:"reason"`
	RefundAmount    Money  `json:"refundAmount"`
}

type balanceRequest struct {
	ProfileID string `json:"profileId"`
}

type balanceResponse struct {
	Name      string           `json:"name"`
	ProfileID string           `json:"profileId"`
	Accounts  []balanceAccount `json:"accounts"`
}

type balanceAccount struct {
	Type      string         `json:"type"`
	Currency  string         `json:"currency"`
	AccountNo string         `json:"accountNo"`
	Balance   balanceAmounts `json:"balance"`
}

type balanceAmounts struct {
	Available string `json:"available"`
	Reserved  string `json:"reserved"`
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

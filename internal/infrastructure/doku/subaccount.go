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
// account name. PartnerReferenceNo is the caller's idempotency key.
func (c *SubAccountClient) TransferInquiry(ctx context.Context, req TransferInquiryRequest) (*TransferInquiryResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	if req.PartnerReferenceNo == "" {
		return nil, fmt.Errorf("doku transfer inquiry requires a partner reference no (the idempotency key)")
	}

	parsed, replayed, err := c.transfer(ctx, subAccountInquiryPath, req.body())
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
func (c *SubAccountClient) TransferPayment(ctx context.Context, req TransferPaymentRequest) (*TransferPaymentResult, error) {
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
	parsed, replayed, err := c.transfer(ctx, subAccountPaymentPath, body)
	if err != nil {
		return nil, err
	}

	return &TransferPaymentResult{
		ReferenceNo:        parsed.ReferenceNo,
		PartnerReferenceNo: firstNonEmpty(parsed.PartnerReferenceNo, req.PartnerReferenceNo),
		Replayed:           replayed,
	}, nil
}

// TransactionStatus reads the latest status of a transfer by its partner
// reference no.
func (c *SubAccountClient) TransactionStatus(ctx context.Context, partnerReferenceNo string) (*TransactionStatusResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}

	var parsed transactionStatusResponse
	err := c.postJSON(ctx, subAccountStatusPath, struct {
		PartnerReferenceNo string `json:"partnerReferenceNo"`
	}{PartnerReferenceNo: partnerReferenceNo}, &parsed)
	if err != nil {
		return nil, err
	}

	return &TransactionStatusResult{
		PartnerReferenceNo: firstNonEmpty(parsed.OriginalPartnerReferenceNo, partnerReferenceNo),
		ReferenceNo:        parsed.OriginalReferenceNo,
		LatestStatus:       parsed.LatestTransactionStatus,
		StatusDescription:  parsed.TransactionStatusDesc,
	}, nil
}

// BalanceInquiry reads the available balance of a Mitra's sub-account profile.
func (c *SubAccountClient) BalanceInquiry(ctx context.Context, profileID string) (*BalanceResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}

	var parsed balanceResponse
	err := c.postJSON(ctx, subAccountBalancePath, struct {
		ProfileID string `json:"profileId"`
	}{ProfileID: profileID}, &parsed)
	if err != nil {
		return nil, err
	}

	return &BalanceResult{
		Currency:  parsed.AvailableBalance.Currency,
		Available: parsed.AvailableBalance.Value,
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
	Replayed           bool
}

type TransactionStatusResult struct {
	PartnerReferenceNo string
	ReferenceNo        string
	LatestStatus       string
	StatusDescription  string
}

type BalanceResult struct {
	Currency  string
	Available string
}

func (c *SubAccountClient) requireConfigured() error {
	if c.cfg.ClientID == "" || c.cfg.SecretKey == "" {
		return fmt.Errorf("doku sub-account is not configured: DOKU_CLIENT_ID and DOKU_SECRET_KEY are required")
	}
	return nil
}

// transfer posts a transfer call. A 409 that carries a reference is the
// original transfer answering a repeated partnerReferenceNo, so it is a replay.
func (c *SubAccountClient) transfer(ctx context.Context, path string, payload any) (transferResponse, bool, error) {
	status, raw, err := c.call(ctx, path, payload)
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
		return transferResponse{}, false, fmt.Errorf("doku sub-account %s returned HTTP %d: %s", path, status, truncate(string(raw), 500))
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return transferResponse{}, false, fmt.Errorf("parse doku sub-account %s response: %w", path, err)
	}
	if parsed.ReferenceNo == "" {
		return transferResponse{}, false, fmt.Errorf("doku sub-account %s response missing referenceNo", path)
	}

	return parsed, false, nil
}

func (c *SubAccountClient) postJSON(ctx context.Context, path string, payload any, out any) error {
	status, raw, err := c.call(ctx, path, payload)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("doku sub-account %s returned HTTP %d: %s", path, status, truncate(string(raw), 500))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parse doku sub-account %s response: %w", path, err)
	}
	return nil
}

func (c *SubAccountClient) call(ctx context.Context, path string, payload any) (int, []byte, error) {
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
	httpReq.Header.Set("X-EXTERNAL-ID", c.externalID())
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
		return "", fmt.Errorf("parse doku token response: %w", err)
	}
	if parsed.Token == "" {
		return "", fmt.Errorf("doku token response missing token")
	}
	return parsed.Token, nil
}

// externalID returns a numeric X-EXTERNAL-ID. DOKU requires it unique per day,
// and the nanosecond clock is unique across the calls this client makes.
func (c *SubAccountClient) externalID() string {
	return strconv.FormatInt(c.now().UnixNano(), 10)
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
		Amount: transferAmount{
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
	PartnerReferenceNo       string         `json:"partnerReferenceNo"`
	Type                     string         `json:"type"`
	Amount                   transferAmount `json:"amount"`
	Channel                  string         `json:"channel"`
	FromAccount              string         `json:"fromAccount"`
	BeneficiaryBankCode      string         `json:"beneficiaryBankCode"`
	BeneficiaryAccountNumber string         `json:"beneficiaryAccountNumber"`
	Remark                   string         `json:"remark"`
}

type transferPaymentBody struct {
	transferBody
	ReferenceNo            string `json:"referenceNo"`
	BeneficiaryAccountName string `json:"beneficiaryAccountName"`
}

type transferAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type transferResponse struct {
	ReferenceNo            string `json:"referenceNo"`
	PartnerReferenceNo     string `json:"partnerReferenceNo"`
	BeneficiaryAccountName string `json:"beneficiaryAccountName"`
}

type transactionStatusResponse struct {
	OriginalPartnerReferenceNo string `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string `json:"originalReferenceNo"`
	LatestTransactionStatus    string `json:"latestTransactionStatus"`
	TransactionStatusDesc      string `json:"transactionStatusDesc"`
}

type balanceResponse struct {
	AvailableBalance transferAmount `json:"availableBalance"`
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

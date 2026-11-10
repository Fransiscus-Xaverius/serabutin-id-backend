package doku

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	snapTokenPath    = "/authorization/v1/access-token/b2b"
	snapInquiryPath  = "/snap/v1.1/emoney/bank-account-inquiry"
	snapTransferPath = "/snap/v1.1/emoney/transfer-bank"
	snapStatusPath   = "/snap/v1.1/qr/qr-mpm-status"
	snapBalancePath  = "/snap/v1.1/balance-inquiry"

	snapTimestampLayout = "2006-01-02T15:04:05-07:00"
	snapTokenLayout     = "2006-01-02T15:04:05Z"
	snapCurrency        = "IDR"
	snapServiceCode     = "43"
	snapMaxResponse     = 1 << 20
	// CHANNEL-ID is the host-to-host channel, while additionalInfo.channelCode
	// carries the payout rail (07 bank deposit, 11 BI-FAST).
	snapHeaderChannel = "H2H"
)

var snapZone = time.FixedZone("WIB", 7*60*60)

// PayoutConfig is the Kirim DOKU SNAP (Domestic Payouts) configuration. It is
// separate from Config so the checkout client never holds the payout private
// key. Secrets are env-only.
type PayoutConfig struct {
	ClientID      string
	SecretKey     string
	PrivateKey    string
	BaseURL       string
	ChannelID     string
	SourceAccount string

	SenderFirstName        string
	SenderLastName         string
	SenderPersonalID       string
	SenderPersonalIDType   string
	SenderCountryCode      string
	CustomerNumberFallback string

	HTTPClient *http.Client
}

type SnapPayoutClient struct {
	cfg        PayoutConfig
	httpClient *http.Client
	now        func() time.Time

	mu           sync.Mutex
	token        string
	tokenExpires time.Time
}

func NewSnapPayoutClient(cfg PayoutConfig) *SnapPayoutClient {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultRequestTimeout}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api-sandbox.doku.com"
	}
	if cfg.ChannelID == "" {
		cfg.ChannelID = "11"
	}
	if cfg.SenderCountryCode == "" {
		cfg.SenderCountryCode = "ID"
	}
	return &SnapPayoutClient{cfg: cfg, httpClient: httpClient, now: time.Now}
}

// TransferInquiry validates the beneficiary account and returns the sessionId
// that transfer-bank requires. It is the SNAP account-inquiry call.
func (c *SnapPayoutClient) TransferInquiry(ctx context.Context, externalID string, req TransferInquiryRequest) (*TransferInquiryResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	if req.PartnerReferenceNo == "" {
		return nil, fmt.Errorf("doku snap inquiry requires a partner reference no")
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("doku snap inquiry requires a positive amount")
	}
	if req.BeneficiaryBankCode == "" || req.BeneficiaryAccountNumber == "" {
		return nil, fmt.Errorf("doku snap inquiry requires the beneficiary bank code and account number")
	}
	name, err := normalizeBeneficiaryName(req.BeneficiaryName)
	if err != nil {
		return nil, err
	}
	customerNumber, err := c.customerNumber(req.CustomerNumber)
	if err != nil {
		return nil, err
	}

	body := snapInquiryBody{
		PartnerReferenceNo:       req.PartnerReferenceNo,
		CustomerNumber:           customerNumber,
		Amount:                   Money{Value: fmt.Sprintf("%d.00", req.Amount), Currency: snapCurrency},
		BeneficiaryAccountNumber: req.BeneficiaryAccountNumber,
		AdditionalInfo: snapInquiryInfo{
			ChannelCode:            c.cfg.ChannelID,
			BeneficiaryBankCode:    req.BeneficiaryBankCode,
			BeneficiaryAccountName: name,
			SenderCountryCode:      firstNonEmpty(req.SenderCountryCode, c.cfg.SenderCountryCode, "ID"),
		},
	}
	status, raw, err := c.call(ctx, externalID, snapInquiryPath, body)
	if err != nil {
		return nil, err
	}

	var parsed snapInquiryResponse
	if status == http.StatusConflict {
		if err := json.Unmarshal(raw, &parsed); err == nil && (parsed.SessionID != "" || parsed.ReferenceNo != "") {
			return c.inquiryResult(parsed, req, true)
		}
	}
	if status < 200 || status >= 300 {
		return nil, snapError(snapInquiryPath, status, raw)
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse doku snap inquiry response")
	}
	return c.inquiryResult(parsed, req, false)
}

func (c *SnapPayoutClient) inquiryResult(parsed snapInquiryResponse, req TransferInquiryRequest, replayed bool) (*TransferInquiryResult, error) {
	// A replay may repeat only part of the original answer. The caller keeps
	// the stored session/reference, so only a fresh inquiry must carry both.
	if !replayed && parsed.SessionID == "" {
		return nil, fmt.Errorf("doku snap inquiry response missing sessionId")
	}
	if !replayed && parsed.ReferenceNo == "" {
		return nil, fmt.Errorf("doku snap inquiry response missing referenceNo")
	}
	name := firstNonEmpty(parsed.BeneficiaryAccountName, req.BeneficiaryName)
	if name == "" {
		return nil, fmt.Errorf("doku snap inquiry response missing beneficiaryAccountName")
	}
	return &TransferInquiryResult{
		ReferenceNo:            parsed.ReferenceNo,
		PartnerReferenceNo:     firstNonEmpty(parsed.PartnerReferenceNo, req.PartnerReferenceNo),
		BeneficiaryAccountName: name,
		SessionID:              parsed.SessionID,
		Replayed:               replayed,
	}, nil
}

// TransferPayment sends the money after TransferInquiry returned a sessionId.
// req.ReferenceNo and req.SessionID both accept the session id.
func (c *SnapPayoutClient) TransferPayment(ctx context.Context, externalID string, req TransferPaymentRequest) (*TransferPaymentResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	if req.PartnerReferenceNo == "" {
		return nil, fmt.Errorf("doku snap transfer requires a partner reference no")
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("doku snap transfer requires a positive amount")
	}
	sessionID := firstNonEmpty(req.SessionID, req.ReferenceNo)
	if sessionID == "" {
		return nil, fmt.Errorf("doku snap transfer requires the inquiry sessionId")
	}
	if req.BeneficiaryBankCode == "" || req.BeneficiaryAccountNumber == "" {
		return nil, fmt.Errorf("doku snap transfer requires the beneficiary bank code and account number")
	}
	customerNumber, err := c.customerNumber(req.CustomerNumber)
	if err != nil {
		return nil, err
	}
	phone, err := c.beneficiaryPhone(req.BeneficiaryPhone)
	if err != nil {
		return nil, err
	}
	firstName, lastName, err := splitBeneficiaryName(req.BeneficiaryAccountName)
	if err != nil {
		return nil, err
	}
	if err := c.requireSender(); err != nil {
		return nil, err
	}
	bankName := strings.TrimSpace(req.BeneficiaryBankName)
	if bankName == "" {
		bankName = req.BeneficiaryBankCode
	}

	body := snapTransferBody{
		PartnerReferenceNo:       req.PartnerReferenceNo,
		CustomerNumber:           customerNumber,
		BeneficiaryAccountNumber: req.BeneficiaryAccountNumber,
		BeneficiaryBankCode:      req.BeneficiaryBankCode,
		Amount:                   Money{Value: fmt.Sprintf("%d.00", req.Amount), Currency: snapCurrency},
		SessionID:                sessionID,
		AdditionalInfo: snapTransferInfo{
			ChannelCode:            c.cfg.ChannelID,
			BeneficiaryFirstName:   firstName,
			BeneficiaryLastName:    lastName,
			BeneficiaryPhoneNumber: phone,
			BeneficiaryBankName:    bankName,
			BeneficiaryAccountName: req.BeneficiaryAccountName,
			SenderCountryCode:      c.cfg.SenderCountryCode,
			SenderFirstName:        c.cfg.SenderFirstName,
			SenderLastName:         c.cfg.SenderLastName,
			SenderPersonalID:       c.cfg.SenderPersonalID,
			SenderPersonalIDType:   c.cfg.SenderPersonalIDType,
			Remark:                 firstNonEmpty(req.Remark, "Serabutin payout"),
		},
	}
	status, raw, err := c.call(ctx, externalID, snapTransferPath, body)
	if err != nil {
		return nil, err
	}

	var parsed snapTransferResponse
	if status == http.StatusConflict {
		if err := json.Unmarshal(raw, &parsed); err == nil && parsed.ReferenceNo != "" {
			return c.transferResult(parsed, req, true)
		}
	}
	if status < 200 || status >= 300 {
		return nil, snapError(snapTransferPath, status, raw)
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse doku snap transfer response")
	}
	return c.transferResult(parsed, req, false)
}

func (c *SnapPayoutClient) transferResult(parsed snapTransferResponse, req TransferPaymentRequest, replayed bool) (*TransferPaymentResult, error) {
	if parsed.ReferenceNo == "" {
		return nil, fmt.Errorf("doku snap transfer response missing referenceNo")
	}
	return &TransferPaymentResult{
		ReferenceNo:        parsed.ReferenceNo,
		PartnerReferenceNo: firstNonEmpty(parsed.PartnerReferenceNo, req.PartnerReferenceNo),
		ReferenceNumber:    parsed.ReferenceNumber,
		TransactionDate:    parsed.TransactionDate,
		Replayed:           replayed,
	}, nil
}

// TransactionStatus is the SNAP check-status call. DOKU's answer is the only
// source of the final payout state.
func (c *SnapPayoutClient) TransactionStatus(ctx context.Context, externalID string, req TransactionStatusRequest) (*TransactionStatusResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	if req.PartnerReferenceNo == "" {
		return nil, fmt.Errorf("doku snap check-status requires a partner reference no")
	}
	body := snapStatusBody{
		OriginalPartnerReferenceNo: req.PartnerReferenceNo,
		ServiceCode:                snapServiceCode,
		BeneficiaryAccountNumber:   req.BeneficiaryAccountNumber,
		BeneficiaryBankCode:        req.BeneficiaryBankCode,
	}
	status, raw, err := c.call(ctx, externalID, snapStatusPath, body)
	if err != nil {
		return nil, err
	}
	// Only DOKU's own 404 response code proves that no transfer exists. A
	// bare HTTP 404 from a proxy or a wrong path must stay a transient error,
	// because this answer is what authorises a re-drive.
	if hasNotFoundCode(raw) {
		return nil, ErrTransactionNotFound
	}
	if status < 200 || status >= 300 {
		return nil, snapError(snapStatusPath, status, raw)
	}

	var parsed snapStatusResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse doku snap check-status response")
	}
	if parsed.LatestStatus == "" {
		return nil, fmt.Errorf("doku snap check-status response missing latestTransactionStatus")
	}
	if parsed.OriginalPartnerReferenceNo != "" && parsed.OriginalPartnerReferenceNo != req.PartnerReferenceNo {
		return nil, fmt.Errorf("doku snap check-status response does not match partner reference no %q", req.PartnerReferenceNo)
	}
	return &TransactionStatusResult{
		PartnerReferenceNo:  firstNonEmpty(parsed.OriginalPartnerReferenceNo, req.PartnerReferenceNo),
		OriginalReferenceNo: parsed.OriginalReferenceNo,
		SessionID:           parsed.SessionID,
		LatestStatus:        parsed.LatestStatus,
		LatestStatusDesc:    firstNonEmpty(parsed.TransactionStatusDesc, parsed.OriginalResponseMessage),
		TransactionDate:     parsed.TransactionDate,
		Amount:              parsed.Amount,
	}, nil
}

// BalanceInquiry reads the merchant payout account balance. The legacy
// BalanceResult shape is kept so the payout usecase can apply its existing
// settled-balance gate.
func (c *SnapPayoutClient) BalanceInquiry(ctx context.Context, externalID string, accountNo string) (*BalanceResult, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	if accountNo == "" {
		return nil, fmt.Errorf("doku snap balance inquiry requires the merchant account number")
	}
	body := snapBalanceBody{
		PartnerReferenceNo: "BAL-" + externalID,
		AccountNo:          accountNo,
	}
	status, raw, err := c.call(ctx, externalID, snapBalancePath, body)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, snapError(snapBalancePath, status, raw)
	}

	var parsed snapBalanceResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse doku snap balance response")
	}
	available := firstNonEmpty(parsed.AccountInfos.Available.Value, parsed.AccountInfos.AvailableAlt.Value)
	if available == "" {
		return nil, fmt.Errorf("doku snap balance response missing available balance")
	}
	currency := firstNonEmpty(parsed.AccountInfos.Available.Currency, parsed.AccountInfos.AvailableAlt.Currency, snapCurrency)
	return &BalanceResult{
		ProfileID: accountNo,
		Name:      parsed.Name,
		Accounts: []BalanceAccount{{
			Type:      BalanceAccountMerchantIDR,
			Currency:  currency,
			AccountNo: parsed.AccountNo,
			Available: available,
			Reserved:  parsed.AccountInfos.HoldAmount.Value,
		}},
	}, nil
}

func (c *SnapPayoutClient) requireConfigured() error {
	var missing []string
	if c.cfg.ClientID == "" {
		missing = append(missing, "DOKU_SNAP_CLIENT_ID or DOKU_CLIENT_ID")
	}
	if c.cfg.SecretKey == "" {
		missing = append(missing, "DOKU_SNAP_CLIENT_SECRET or DOKU_SECRET_KEY")
	}
	if strings.TrimSpace(c.cfg.PrivateKey) == "" {
		missing = append(missing, "DOKU_SNAP_PRIVATE_KEY")
	}
	if c.cfg.BaseURL == "" {
		missing = append(missing, "DOKU_BASE_URL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("doku snap payout is not configured: %s must be set", strings.Join(missing, ", "))
	}
	return nil
}

func (c *SnapPayoutClient) requireSender() error {
	var missing []string
	if strings.TrimSpace(c.cfg.SenderFirstName) == "" {
		missing = append(missing, "DOKU_PAYOUT_SENDER_FIRST_NAME")
	}
	if strings.TrimSpace(c.cfg.SenderLastName) == "" {
		missing = append(missing, "DOKU_PAYOUT_SENDER_LAST_NAME")
	}
	if strings.TrimSpace(c.cfg.SenderPersonalID) == "" {
		missing = append(missing, "DOKU_PAYOUT_SENDER_PERSONAL_ID")
	}
	if strings.TrimSpace(c.cfg.SenderPersonalIDType) == "" {
		missing = append(missing, "DOKU_PAYOUT_SENDER_PERSONAL_ID_TYPE")
	}
	if len(missing) > 0 {
		return fmt.Errorf("doku snap transfer is missing sender identity: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (c *SnapPayoutClient) customerNumber(explicit string) (string, error) {
	value := NormalizePhone(firstNonEmpty(explicit, c.cfg.CustomerNumberFallback))
	if value == "" {
		return "", fmt.Errorf("doku snap payout requires customerNumber (the Mitra phone or DOKU_PAYOUT_CUSTOMER_NUMBER)")
	}
	return value, nil
}

func (c *SnapPayoutClient) beneficiaryPhone(explicit string) (string, error) {
	value := NormalizePhone(firstNonEmpty(explicit, c.cfg.CustomerNumberFallback))
	if value == "" {
		return "", fmt.Errorf("doku snap transfer requires beneficiaryPhoneNumber")
	}
	return value, nil
}

// call signs and sends one SNAP request. A 401 clears the cached token and is
// retried once, so an early expiry does not fail a release.
func (c *SnapPayoutClient) call(ctx context.Context, externalID string, path string, payload any) (int, []byte, error) {
	if !isNumeric(externalID) {
		return 0, nil, fmt.Errorf("doku snap %s requires a numeric external id", path)
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return 0, nil, err
	}
	status, raw, err := c.callWithToken(ctx, externalID, path, payload, token)
	if status == http.StatusUnauthorized {
		c.clearToken()
		token, tokenErr := c.accessToken(ctx)
		if tokenErr != nil {
			return 0, nil, tokenErr
		}
		return c.callWithToken(ctx, externalID, path, payload, token)
	}
	return status, raw, err
}

func (c *SnapPayoutClient) callWithToken(ctx context.Context, externalID string, path string, payload any, token string) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal doku snap %s request: %w", path, err)
	}
	timestamp := c.now().In(snapZone).Format(snapTimestampLayout)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("build doku snap %s request: %w", path, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("X-PARTNER-ID", c.cfg.ClientID)
	httpReq.Header.Set("X-EXTERNAL-ID", externalID)
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("CHANNEL-ID", snapHeaderChannel)
	httpReq.Header.Set("X-SIGNATURE", SignSHA512(c.cfg.SecretKey, http.MethodPost, path, token, string(body), timestamp))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, nil, fmt.Errorf("call doku snap %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, snapMaxResponse))
	if err != nil {
		return 0, nil, fmt.Errorf("read doku snap %s response: %w", path, err)
	}
	return resp.StatusCode, raw, nil
}

func (c *SnapPayoutClient) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Before(c.tokenExpires) {
		return c.token, nil
	}
	token, expiresIn, err := c.fetchToken(ctx)
	if err != nil {
		return "", err
	}
	c.token = token
	if expiresIn <= 0 {
		expiresIn = 900
	}
	refreshAfter := time.Duration(expiresIn)*time.Second - 60*time.Second
	if refreshAfter < time.Second {
		refreshAfter = time.Duration(expiresIn) * time.Second / 2
	}
	c.tokenExpires = c.now().Add(refreshAfter)
	return token, nil
}

func (c *SnapPayoutClient) clearToken() {
	c.mu.Lock()
	c.token = ""
	c.tokenExpires = time.Time{}
	c.mu.Unlock()
}

func (c *SnapPayoutClient) fetchToken(ctx context.Context) (string, int, error) {
	privateKey, err := parseRSAPrivateKey(c.cfg.PrivateKey)
	if err != nil {
		return "", 0, err
	}
	timestamp := c.now().UTC().Format(snapTokenLayout)
	stringToSign := c.cfg.ClientID + "|" + timestamp
	signature, err := signRSA(privateKey, stringToSign)
	if err != nil {
		return "", 0, fmt.Errorf("sign doku snap token request: %w", err)
	}

	body, err := json.Marshal(snapTokenRequest{GrantType: "client_credentials"})
	if err != nil {
		return "", 0, fmt.Errorf("marshal doku snap token request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.BaseURL, "/")+snapTokenPath, bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("build doku snap token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-CLIENT-KEY", c.cfg.ClientID)
	httpReq.Header.Set("X-TIMESTAMP", timestamp)
	httpReq.Header.Set("X-SIGNATURE", signature)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", 0, fmt.Errorf("call doku snap token: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, snapMaxResponse))
	if err != nil {
		return "", 0, fmt.Errorf("read doku snap token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, snapError(snapTokenPath, resp.StatusCode, raw)
	}
	var parsed snapTokenResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", 0, fmt.Errorf("parse doku snap token response")
	}
	if parsed.AccessToken == "" {
		return "", 0, fmt.Errorf("doku snap token response missing accessToken")
	}
	return parsed.AccessToken, int(parsed.ExpiresIn), nil
}

func parseRSAPrivateKey(value string) (*rsa.PrivateKey, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, `\n`, "\n"))
	if value == "" {
		return nil, fmt.Errorf("DOKU_SNAP_PRIVATE_KEY is empty")
	}
	block, _ := pem.Decode([]byte(value))
	if block == nil {
		if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
			block, _ = pem.Decode(decoded)
		}
	}
	if block == nil {
		// A path is also accepted, so the key can stay in a mounted file.
		if data, err := os.ReadFile(value); err == nil {
			block, _ = pem.Decode(data)
		}
	}
	if block == nil {
		return nil, fmt.Errorf("DOKU_SNAP_PRIVATE_KEY is neither PEM, base64 PEM, nor a readable PEM file")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("DOKU_SNAP_PRIVATE_KEY is not a PKCS1 or PKCS8 RSA key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("DOKU_SNAP_PRIVATE_KEY is not an RSA key")
	}
	return key, nil
}

// ValidatePayoutPrivateKey reports whether the configured value can be loaded
// as an RSA private key. The error never contains the key.
func ValidatePayoutPrivateKey(value string) error {
	_, err := parseRSAPrivateKey(value)
	return err
}

func signRSA(key *rsa.PrivateKey, value string) (string, error) {
	sum := sha256.Sum256([]byte(value))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// normalizeBeneficiaryName enforces DOKU's 64-character limit on runes, not
// bytes, so a non-Latin name that fits the visual limit is not rejected.
func normalizeBeneficiaryName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", fmt.Errorf("doku snap inquiry requires the beneficiary account name")
	}
	if utf8.RuneCountInString(name) > 64 {
		return "", fmt.Errorf("doku snap inquiry beneficiary name exceeds the 64 character limit")
	}
	return name, nil
}

func splitBeneficiaryName(name string) (string, string, error) {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "", "", fmt.Errorf("doku snap transfer requires beneficiaryFirstName and beneficiaryLastName")
	}
	first := fields[0]
	last := strings.Join(fields[1:], " ")
	if last == "" {
		// SNAP requires both fields. A single-word name is repeated rather
		// than sent with an empty last name.
		last = first
	}
	if utf8.RuneCountInString(first) > 64 || utf8.RuneCountInString(last) > 64 {
		return "", "", fmt.Errorf("beneficiary name parts exceed the 64 character DOKU limit")
	}
	return first, last, nil
}

func hasNotFoundCode(raw []byte) bool {
	var parsed struct {
		ResponseCode string `json:"responseCode"`
	}
	if json.Unmarshal(raw, &parsed) != nil {
		return false
	}
	return strings.HasPrefix(parsed.ResponseCode, "404")
}

func snapError(path string, status int, raw []byte) error {
	var parsed struct {
		ResponseCode    string `json:"responseCode"`
		ResponseMessage string `json:"responseMessage"`
	}
	if json.Unmarshal(raw, &parsed) == nil {
		// The provider message can echo beneficiary data, so only the
		// response code and HTTP status are safe to keep.
		if parsed.ResponseCode != "" {
			return fmt.Errorf("doku snap %s returned HTTP %d (responseCode %s)", path, status, parsed.ResponseCode)
		}
	}
	return fmt.Errorf("doku snap %s returned HTTP %d", path, status)
}

type snapTokenRequest struct {
	GrantType string `json:"grantType"`
}

type snapTokenResponse struct {
	ResponseCode string        `json:"responseCode"`
	AccessToken  string        `json:"accessToken"`
	TokenType    string        `json:"tokenType"`
	ExpiresIn    snapExpiresIn `json:"expiresIn"`
}

// snapExpiresIn accepts both the numeric and string forms DOKU has published.
type snapExpiresIn int

func (e *snapExpiresIn) UnmarshalJSON(data []byte) error {
	raw := strings.Trim(strings.TrimSpace(string(data)), `"`)
	if raw == "" || raw == "null" {
		*e = 0
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("expiresIn is not an integer")
	}
	*e = snapExpiresIn(value)
	return nil
}

type snapInquiryBody struct {
	PartnerReferenceNo       string          `json:"partnerReferenceNo"`
	CustomerNumber           string          `json:"customerNumber"`
	Amount                   Money           `json:"amount"`
	BeneficiaryAccountNumber string          `json:"beneficiaryAccountNumber"`
	AdditionalInfo           snapInquiryInfo `json:"additionalInfo"`
}

type snapInquiryInfo struct {
	ChannelCode            string `json:"channelCode"`
	BeneficiaryBankCode    string `json:"beneficiaryBankCode"`
	BeneficiaryAccountName string `json:"beneficiaryAccountName"`
	SenderCountryCode      string `json:"senderCountryCode"`
}

type snapInquiryResponse struct {
	ResponseCode           string `json:"responseCode"`
	ResponseMessage        string `json:"responseMessage"`
	ReferenceNo            string `json:"referenceNo"`
	PartnerReferenceNo     string `json:"partnerReferenceNo"`
	BeneficiaryAccountName string `json:"beneficiaryAccountName"`
	SessionID              string `json:"sessionId"`
}

type snapTransferBody struct {
	PartnerReferenceNo       string           `json:"partnerReferenceNo"`
	CustomerNumber           string           `json:"customerNumber"`
	BeneficiaryAccountNumber string           `json:"beneficiaryAccountNumber"`
	BeneficiaryBankCode      string           `json:"beneficiaryBankCode"`
	Amount                   Money            `json:"amount"`
	SessionID                string           `json:"sessionId"`
	AdditionalInfo           snapTransferInfo `json:"additionalInfo"`
}

type snapTransferInfo struct {
	ChannelCode            string `json:"channelCode"`
	BeneficiaryFirstName   string `json:"beneficiaryFirstName"`
	BeneficiaryLastName    string `json:"beneficiaryLastName"`
	BeneficiaryPhoneNumber string `json:"beneficiaryPhoneNumber"`
	BeneficiaryBankName    string `json:"beneficiaryBankName,omitempty"`
	BeneficiaryAccountName string `json:"beneficiaryAccountName"`
	SenderCountryCode      string `json:"senderCountryCode"`
	SenderFirstName        string `json:"senderFirstName"`
	SenderLastName         string `json:"senderLastName"`
	SenderPersonalID       string `json:"senderPersonalId"`
	SenderPersonalIDType   string `json:"senderPersonalIdType"`
	Remark                 string `json:"remark,omitempty"`
}

type snapTransferResponse struct {
	ResponseCode       string `json:"responseCode"`
	ResponseMessage    string `json:"responseMessage"`
	ReferenceNo        string `json:"referenceNo"`
	PartnerReferenceNo string `json:"partnerReferenceNo"`
	ReferenceNumber    string `json:"referenceNumber"`
	TransactionDate    string `json:"transactionDate"`
	AdditionalInfo     struct {
		SessionID string `json:"sessionId"`
		Amount    string `json:"amount"`
	} `json:"additionalInfo"`
}

type snapStatusBody struct {
	OriginalPartnerReferenceNo string `json:"originalPartnerReferenceNo"`
	ServiceCode                string `json:"serviceCode"`
	BeneficiaryAccountNumber   string `json:"beneficiaryAccountNumber"`
	BeneficiaryBankCode        string `json:"beneficiaryBankCode"`
}

type snapStatusResponse struct {
	ResponseCode               string `json:"responseCode"`
	ResponseMessage            string `json:"responseMessage"`
	OriginalPartnerReferenceNo string `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string `json:"originalReferenceNo"`
	SessionID                  string `json:"sessionId"`
	ServiceCode                string `json:"serviceCode"`
	LatestStatus               string `json:"latestTransactionStatus"`
	TransactionStatusDesc      string `json:"transactionStatusDesc"`
	OriginalResponseMessage    string `json:"originalResponseMessage"`
	TransactionDate            string `json:"transactionDate"`
	Amount                     Money  `json:"amount"`
}

type snapBalanceBody struct {
	PartnerReferenceNo string `json:"partnerReferenceNo"`
	AccountNo          string `json:"accountNo"`
}

type snapBalanceResponse struct {
	ResponseCode string `json:"responseCode"`
	AccountNo    string `json:"accountNo"`
	Name         string `json:"name"`
	AccountInfos struct {
		HoldAmount   snapAmount `json:"holdAmount"`
		Available    snapAmount `json:"avaliableBalance"`
		AvailableAlt snapAmount `json:"availableBalance"`
	} `json:"accountInfos"`
}

type snapAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

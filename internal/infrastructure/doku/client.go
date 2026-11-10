package doku

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/payment"
)

const (
	checkoutPath          = "/checkout/v1/payment"
	timestampLayout       = "2006-01-02T15:04:05Z"
	expiredDateLayout     = "20060102150405"
	signaturePrefix       = "HMACSHA256="
	defaultNotification   = "/api/v1/payments/doku/webhook"
	defaultRequestTimeout = 15 * time.Second
	// NotificationTimestampDrift is the replay guard for inbound
	// notifications: DOKU documents a tolerance of 300 seconds around
	// Request-Timestamp (references 06 and 07 of the DOKU skill).
	NotificationTimestampDrift = 300 * time.Second
	// DOKU caps invoice numbers at 64 characters, or 30 when a card
	// channel takes part (acquirer rule; references 01 and 07).
	invoiceNumberLimit     = 64
	cardInvoiceNumberLimit = 30
	dokuCardChannelName    = "CREDIT_CARD"
)

// Config is the DOKU Checkout (non-SNAP) configuration. Secrets must come
// from the environment; nothing here is persisted.
type Config struct {
	ClientID         string
	SecretKey        string
	BaseURL          string
	NotificationPath string
	// OverrideNotificationURL lets a single request point DOKU at another
	// host (e.g. a local tunnel) while keeping the same URL path. DOKU
	// requires the path to match the configured Notification URL.
	OverrideNotificationURL string
	HTTPClient              *http.Client
}

type Client struct {
	cfg        Config
	httpClient *http.Client
	now        func() time.Time
}

func NewClient(cfg Config) *Client {
	notificationPath := cfg.NotificationPath
	if notificationPath == "" {
		notificationPath = defaultNotification
	}
	cfg.NotificationPath = notificationPath

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultRequestTimeout}
	}

	return &Client{cfg: cfg, httpClient: httpClient, now: time.Now}
}

func (c *Client) Name() string {
	return payment.ProviderDOKU
}

func (c *Client) CreateCheckout(ctx context.Context, req payment.CreateCheckoutRequest) (*payment.CreateCheckoutResult, error) {
	if c.cfg.ClientID == "" || c.cfg.SecretKey == "" {
		return nil, fmt.Errorf("doku checkout is not configured: DOKU_CLIENT_ID and DOKU_SECRET_KEY are required")
	}

	requestID := req.RequestID
	if requestID == "" {
		return nil, fmt.Errorf("doku checkout requires a request id (the idempotency key)")
	}
	timestamp := c.now().UTC().Format(timestampLayout)

	checkout, err := c.checkoutBody(req)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(checkout)
	if err != nil {
		return nil, fmt.Errorf("marshal doku checkout request: %w", err)
	}

	signature := Sign(c.cfg.ClientID, requestID, timestamp, checkoutPath, Digest(body), c.cfg.SecretKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.BaseURL, "/")+checkoutPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build doku checkout request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Client-Id", c.cfg.ClientID)
	httpReq.Header.Set("Request-Id", requestID)
	httpReq.Header.Set("Request-Timestamp", timestamp)
	httpReq.Header.Set("Signature", signaturePrefix+signature)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call doku checkout: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusConflict {
		// DOKU answers a duplicate Request-Id with 409 and the original
		// response body. That is a replay, not a failure.
		var replay checkoutResponse
		if err := json.Unmarshal(raw, &replay); err == nil && replay.Response.Payment.URL != "" {
			return c.checkoutResult(replay, requestID, req.ExpiryMinutes, true), nil
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("doku checkout returned HTTP %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}

	var parsed checkoutResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse doku checkout response: %w", err)
	}
	if parsed.Response.Payment.URL == "" {
		return nil, fmt.Errorf("doku checkout response missing payment url")
	}

	return c.checkoutResult(parsed, requestID, req.ExpiryMinutes, false), nil
}

func (c *Client) checkoutResult(parsed checkoutResponse, requestID string, expiryMinutes int, replayed bool) *payment.CreateCheckoutResult {
	expiresAt := parseExpiredDate(parsed.Response.Payment.ExpiredDate)
	if expiresAt.IsZero() {
		minutes := expiryMinutes
		if minutes <= 0 {
			minutes = 60
		}
		expiresAt = c.now().Add(time.Duration(minutes) * time.Minute)
	}

	return &payment.CreateCheckoutResult{
		PaymentURL:        parsed.Response.Payment.URL,
		ProviderRequestID: requestID,
		ProviderSessionID: parsed.Response.Order.SessionID,
		ExpiresAt:         expiresAt,
		Replayed:          replayed,
	}
}

func (c *Client) VerifyNotification(rawBody []byte, headers map[string]string) error {
	if c.cfg.ClientID == "" || c.cfg.SecretKey == "" {
		return fmt.Errorf("%w: doku is not configured", payment.ErrInvalidSignature)
	}

	clientID := header(headers, "Client-Id")
	requestID := header(headers, "Request-Id")
	timestamp := header(headers, "Request-Timestamp")
	signatureHeader := header(headers, "Signature")
	if clientID == "" || requestID == "" || timestamp == "" || signatureHeader == "" {
		return fmt.Errorf("%w: missing DOKU notification headers", payment.ErrInvalidSignature)
	}
	if !hmac.Equal([]byte(clientID), []byte(c.cfg.ClientID)) {
		return fmt.Errorf("%w: client id mismatch", payment.ErrInvalidSignature)
	}

	// Replay guard: a notification older (or further ahead) than the
	// documented 300 second tolerance is rejected even when its signature
	// is valid, so a captured notification cannot be replayed later.
	sentAt, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fmt.Errorf("%w: notification timestamp is not RFC3339: %q", payment.ErrInvalidSignature, timestamp)
	}
	drift := c.now().Sub(sentAt)
	if drift > NotificationTimestampDrift || drift < -NotificationTimestampDrift {
		return fmt.Errorf("%w: notification timestamp outside the %s tolerance", payment.ErrInvalidSignature, NotificationTimestampDrift)
	}

	signature := strings.TrimSpace(strings.TrimPrefix(signatureHeader, signaturePrefix))
	if signature == "" || signature == signatureHeader {
		return fmt.Errorf("%w: unsupported signature format", payment.ErrInvalidSignature)
	}

	expected := Sign(c.cfg.ClientID, requestID, timestamp, c.cfg.NotificationPath, Digest(rawBody), c.cfg.SecretKey)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return fmt.Errorf("%w: signature mismatch", payment.ErrInvalidSignature)
	}

	return nil
}

func (c *Client) checkoutBody(req payment.CreateCheckoutRequest) (checkoutRequest, error) {
	channel, ok := entity.DOKUChannelName(req.Channel)
	if !ok {
		return checkoutRequest{}, fmt.Errorf("unknown payment channel %q", req.Channel)
	}

	// DOKU rejects long invoice numbers only after the checkout is created,
	// so the limit is enforced here: 64 characters in general, 30 when the
	// card channel takes part (acquirer rule, reference 01 section 4).
	invoiceLimit := invoiceNumberLimit
	if channel == dokuCardChannelName {
		invoiceLimit = cardInvoiceNumberLimit
	}
	if len(req.InvoiceNumber) > invoiceLimit {
		return checkoutRequest{}, fmt.Errorf("invoice number %q exceeds the %d character DOKU limit for channel %s", req.InvoiceNumber, invoiceLimit, channel)
	}

	body := checkoutRequest{
		Order: checkoutOrder{
			Amount:            req.Amount,
			InvoiceNumber:     req.InvoiceNumber,
			Currency:          req.Currency,
			CallbackURL:       req.CallbackURL,
			CallbackURLRsl:    req.CallbackURL,
			CallbackURLCancel: req.CallbackURL,
			AutoRedirect:      true,
			Language:          "ID",
		},
		Payment: checkoutPayment{
			PaymentDueDate:     req.ExpiryMinutes,
			PaymentMethodTypes: []string{channel},
			Type:               "SALE",
		},
		Customer: checkoutCustomer{
			ID:    fmt.Sprintf("%d", req.CustomerID),
			Name:  req.CustomerName,
			Email: req.CustomerEmail,
			Phone: NormalizePhone(req.CustomerPhone),
		},
	}
	if req.Currency == "" {
		body.Order.Currency = "IDR"
	}
	if body.Payment.PaymentDueDate <= 0 {
		body.Payment.PaymentDueDate = 60
	}
	if c.cfg.OverrideNotificationURL != "" {
		// Checkout settles into the main merchant account. The only
		// additional_info field left is the per-request notification
		// override; there is no sub-account routing and no split rule.
		body.AdditionalInfo = &checkoutAdditionalInfo{
			OverrideNotificationURL: c.cfg.OverrideNotificationURL,
		}
	}
	return body, nil
}

type checkoutRequest struct {
	Order          checkoutOrder           `json:"order"`
	Payment        checkoutPayment         `json:"payment"`
	Customer       checkoutCustomer        `json:"customer"`
	AdditionalInfo *checkoutAdditionalInfo `json:"additional_info,omitempty"`
}

// checkoutAdditionalInfo carries the snake_case additional_info object DOKU
// documents for Checkout. Serabutin only uses the notification override:
// checkout settles into the main merchant account, with no WaaS sub-account.
type checkoutAdditionalInfo struct {
	OverrideNotificationURL string `json:"override_notification_url,omitempty"`
}

type checkoutOrder struct {
	Amount            int64  `json:"amount"`
	InvoiceNumber     string `json:"invoice_number"`
	Currency          string `json:"currency"`
	CallbackURL       string `json:"callback_url"`
	CallbackURLRsl    string `json:"callback_url_result"`
	CallbackURLCancel string `json:"callback_url_cancel,omitempty"`
	AutoRedirect      bool   `json:"auto_redirect"`
	Language          string `json:"language"`
}

type checkoutPayment struct {
	PaymentDueDate     int      `json:"payment_due_date"`
	PaymentMethodTypes []string `json:"payment_method_types"`
	Type               string   `json:"type"`
}

type checkoutCustomer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

type checkoutResponse struct {
	Response struct {
		Order struct {
			SessionID string `json:"session_id"`
		} `json:"order"`
		Payment struct {
			URL         string `json:"url"`
			ExpiredDate string `json:"expired_date"`
		} `json:"payment"`
	} `json:"response"`
}

// Digest returns base64(SHA-256(raw body)).
func Digest(rawBody []byte) string {
	sum := sha256.Sum256(rawBody)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Sign returns base64(HMAC-SHA256(secret, stringToSign)) where stringToSign
// carries the component labels, as DOKU documents for non-SNAP request
// headers (verified against the live sandbox on 2026-11-08; the unlabelled
// variant is answered with 400 invalid_signature):
//
//	Client-Id:<id>
//	Request-Id:<requestID>
//	Request-Timestamp:<timestamp>
//	Request-Target:<target>
//	Digest:<digest>
func Sign(clientID, requestID, timestamp, requestTarget, digest, secret string) string {
	stringToSign := strings.Join([]string{
		"Client-Id:" + clientID,
		"Request-Id:" + requestID,
		"Request-Timestamp:" + timestamp,
		"Request-Target:" + requestTarget,
		"Digest:" + digest,
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// NormalizePhone converts an Indonesian local number (08...) into the 628...
// form DOKU expects, strips separators, and keeps an existing 62 prefix.
func NormalizePhone(phone string) string {
	digits := make([]rune, 0, len(phone))
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	normalized := string(digits)
	switch {
	case strings.HasPrefix(normalized, "62"):
		return normalized
	case strings.HasPrefix(normalized, "0"):
		return "62" + normalized[1:]
	case strings.HasPrefix(normalized, "8"):
		return "62" + normalized
	default:
		return normalized
	}
}

func parseExpiredDate(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	wib := time.FixedZone("WIB", 7*60*60)
	parsed, err := time.ParseInLocation(expiredDateLayout, value, wib)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func header(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

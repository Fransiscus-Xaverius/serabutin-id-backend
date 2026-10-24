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

	"github.com/google/uuid"

	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/payment"
)

const (
	checkoutPath          = "/checkout/v1/payment"
	timestampLayout       = "2006-01-02T15:04:05Z"
	expiredDateLayout     = "20060102150405"
	signaturePrefix       = "HMACSHA256="
	defaultNotification   = "/api/v1/payments/doku/webhook"
	defaultRequestTimeout = 15 * time.Second
)

// Config is the DOKU Checkout (non-SNAP) configuration. Secrets must come
// from the environment; nothing here is persisted.
type Config struct {
	ClientID         string
	SecretKey        string
	BaseURL          string
	HubAccountID     string
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

	requestID := uuid.NewString()
	timestamp := c.now().UTC().Format(timestampLayout)

	body, err := json.Marshal(c.checkoutBody(req))
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

	expiresAt := parseExpiredDate(parsed.Response.Payment.ExpiredDate)
	if expiresAt.IsZero() {
		minutes := req.ExpiryMinutes
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
	}, nil
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

func (c *Client) checkoutBody(req payment.CreateCheckoutRequest) checkoutRequest {
	body := checkoutRequest{
		Order: checkoutOrder{
			Amount:         req.Amount,
			InvoiceNumber:  req.InvoiceNumber,
			Currency:       req.Currency,
			CallbackURL:    req.CallbackURL,
			CallbackURLRsl: req.CallbackURL,
			AutoRedirect:   true,
			Language:       "ID",
		},
		Payment: checkoutPayment{
			PaymentDueDate:     req.ExpiryMinutes,
			PaymentMethodTypes: []string{"QRIS"},
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
	if c.cfg.HubAccountID != "" {
		body.AdditionalInfo = &checkoutAdditionalInfo{
			Account: checkoutAccount{ID: c.cfg.HubAccountID},
		}
	}
	if c.cfg.OverrideNotificationURL != "" {
		// DOKU expects the override under additional_info (snake_case);
		// the sub-account routing above uses additionalInfo (camelCase).
		// Both keys can coexist in one request.
		body.Additional = &checkoutAdditional{
			OverrideNotificationURL: c.cfg.OverrideNotificationURL,
		}
	}
	return body
}

type checkoutRequest struct {
	Order          checkoutOrder           `json:"order"`
	Payment        checkoutPayment         `json:"payment"`
	Customer       checkoutCustomer        `json:"customer"`
	AdditionalInfo *checkoutAdditionalInfo `json:"additionalInfo,omitempty"`
	Additional     *checkoutAdditional     `json:"additional_info,omitempty"`
}

type checkoutAdditional struct {
	OverrideNotificationURL string `json:"override_notification_url,omitempty"`
}

type checkoutOrder struct {
	Amount         int64  `json:"amount"`
	InvoiceNumber  string `json:"invoice_number"`
	Currency       string `json:"currency"`
	CallbackURL    string `json:"callback_url"`
	CallbackURLRsl string `json:"callback_url_result"`
	AutoRedirect   bool   `json:"auto_redirect"`
	Language       string `json:"language"`
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

type checkoutAdditionalInfo struct {
	Account checkoutAccount `json:"account"`
}

type checkoutAccount struct {
	ID string `json:"id"`
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

// Sign returns base64(HMAC-SHA256(secret, clientID+"\n"+requestID+"\n"+
// timestamp+"\n"+target+"\n"+digest)).
func Sign(clientID, requestID, timestamp, requestTarget, digest, secret string) string {
	stringToSign := strings.Join([]string{clientID, requestID, timestamp, requestTarget, digest}, "\n")
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

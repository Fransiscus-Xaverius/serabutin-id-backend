package doku

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/payment"
)

// The DOKU Checkout integration is signed with the non-SNAP HMAC scheme:
// base64(sha256(body)) as the digest, then HMAC-SHA256 over the newline
// joined signature components.

const (
	testClientID = "MCH-0001-10791114622547"
	testSecret   = "sk_test_secret"
)

func TestDigest_MatchesKnownValue(t *testing.T) {
	body := []byte(`{"order":{"amount":10000,"invoice_number":"SRB-1-1"}}`)

	assert.Equal(t, "95KQqqnGYunjpPJlmBk/+rKpqVu99muwzSJH9gS5CTA=", Digest(body))
}

func TestSign_MatchesKnownVector(t *testing.T) {
	body := []byte(`{"order":{"amount":10000,"invoice_number":"SRB-1-1"}}`)

	signature := Sign(
		testClientID,
		"fdb69f47-96da-499d-acec-7cdc318ab2fe",
		"2020-08-11T08:45:42Z",
		"/checkout/v1/payment",
		Digest(body),
		testSecret,
	)

	assert.Equal(t, "Fm9vaHiSGbDOQ++HcVnfjTN3OitzftdCI9T1xMmec3o=", signature)
}

func TestNormalizePhone_ConvertsLocalNumbers(t *testing.T) {
	cases := map[string]string{
		"081234567890":  "6281234567890",
		"81234567890":   "6281234567890",
		"628123456789":  "628123456789",
		"0812 3456-789": "628123456789",
	}
	for input, expected := range cases {
		assert.Equal(t, expected, NormalizePhone(input), "input %s", input)
	}
}

func TestCreateCheckout_SendsSignedQRISRequest(t *testing.T) {
	var capturedBody []byte
	var capturedHeader http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		capturedHeader = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"order":{"session_id":"sess-1"},"payment":{"url":"https://checkout.doku.test/pay/1","expired_date":"20261024120000"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		ClientID:     testClientID,
		SecretKey:    testSecret,
		BaseURL:      server.URL,
		HubAccountID: "SAC-1670-1628490169910",
	})

	result, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		BookingID:     42,
		CustomerID:    7,
		InvoiceNumber: "SRB-42-1",
		Amount:        125000,
		Currency:      "IDR",
		CustomerName:  "Budi",
		CustomerEmail: "budi@example.com",
		CustomerPhone: "081234567890",
		CallbackURL:   "http://localhost:5173/payment/return?booking_id=42",
		ExpiryMinutes: 60,
	})

	require.NoError(t, err)
	assert.Equal(t, "https://checkout.doku.test/pay/1", result.PaymentURL)
	assert.Equal(t, "sess-1", result.ProviderSessionID)
	assert.NotEmpty(t, result.ProviderRequestID)

	// Signed request headers.
	assert.Equal(t, testClientID, capturedHeader.Get("Client-Id"))
	assert.NotEmpty(t, capturedHeader.Get("Request-Id"))
	assert.NotEmpty(t, capturedHeader.Get("Request-Timestamp"))
	assert.Contains(t, capturedHeader.Get("Signature"), "HMACSHA256=")

	// Body: QRIS only, correct amount, hub routing, callback.
	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &sent))

	order := sent["order"].(map[string]interface{})
	assert.Equal(t, float64(125000), order["amount"])
	assert.Equal(t, "SRB-42-1", order["invoice_number"])
	assert.Equal(t, "IDR", order["currency"])
	assert.Equal(t, "http://localhost:5173/payment/return?booking_id=42", order["callback_url_result"])

	checkoutPayment := sent["payment"].(map[string]interface{})
	assert.Equal(t, []interface{}{"QRIS"}, checkoutPayment["payment_method_types"])
	assert.Equal(t, float64(60), checkoutPayment["payment_due_date"])

	additionalInfo, ok := sent["additionalInfo"].(map[string]interface{})
	require.True(t, ok, "hub account routing must be sent when a hub account is configured")
	account := additionalInfo["account"].(map[string]interface{})
	assert.Equal(t, "SAC-1670-1628490169910", account["id"])

	customer := sent["customer"].(map[string]interface{})
	assert.Equal(t, "6281234567890", customer["phone"])
}

func TestCreateCheckout_OmitsHubAccountWhenNotConfigured(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/2"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		BookingID:     43,
		InvoiceNumber: "SRB-43-1",
		Amount:        10000,
		ExpiryMinutes: 30,
	})
	require.NoError(t, err)

	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &sent))
	_, hasAdditionalInfo := sent["additionalInfo"]
	assert.False(t, hasAdditionalInfo, "no sub-account routing without a configured hub account")
}

func TestCreateCheckout_SendsOverrideNotificationURLWhenConfigured(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/3"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		ClientID:                testClientID,
		SecretKey:               testSecret,
		BaseURL:                 server.URL,
		OverrideNotificationURL: "https://tunnel.example/api/v1/payments/doku/webhook",
	})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		BookingID:     45,
		InvoiceNumber: "SRB-45-1",
		Amount:        10000,
		ExpiryMinutes: 30,
	})
	require.NoError(t, err)

	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &sent))

	additional, ok := sent["additional_info"].(map[string]interface{})
	require.True(t, ok, "the notification override must be sent under additional_info")
	assert.Equal(t, "https://tunnel.example/api/v1/payments/doku/webhook", additional["override_notification_url"])
}

func TestCreateCheckout_OmitsOverrideNotificationURLWhenNotConfigured(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/4"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-46-1",
		Amount:        10000,
		ExpiryMinutes: 30,
	})
	require.NoError(t, err)

	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &sent))
	_, hasAdditional := sent["additional_info"]
	assert.False(t, hasAdditional)
}

func TestCreateCheckout_FailsWithoutCredentials(t *testing.T) {
	client := NewClient(Config{BaseURL: "https://api-sandbox.doku.com"})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-44-1",
		Amount:        10000,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOKU_CLIENT_ID")
}

func TestVerifyNotification_AcceptsMatchingSignature(t *testing.T) {
	body := []byte(`{"order":{"invoice_number":"SRB-1-1","amount":10000},"transaction":{"status":"SUCCESS"}}`)
	client := NewClient(Config{
		ClientID:         testClientID,
		SecretKey:        testSecret,
		BaseURL:          "https://api-sandbox.doku.com",
		NotificationPath: "/api/v1/payments/doku/webhook",
	})

	signature := Sign(testClientID, "req-1", "2020-08-11T08:45:42Z", "/api/v1/payments/doku/webhook", Digest(body), testSecret)

	err := client.VerifyNotification(body, map[string]string{
		"Client-Id":         testClientID,
		"Request-Id":        "req-1",
		"Request-Timestamp": "2020-08-11T08:45:42Z",
		"Signature":         "HMACSHA256=" + signature,
	})

	require.NoError(t, err)
}

func TestVerifyNotification_RejectsTamperedBodyAndWrongClient(t *testing.T) {
	body := []byte(`{"order":{"invoice_number":"SRB-1-1","amount":10000},"transaction":{"status":"SUCCESS"}}`)
	client := NewClient(Config{
		ClientID:         testClientID,
		SecretKey:        testSecret,
		BaseURL:          "https://api-sandbox.doku.com",
		NotificationPath: "/api/v1/payments/doku/webhook",
	})

	signature := Sign(testClientID, "req-1", "2020-08-11T08:45:42Z", "/api/v1/payments/doku/webhook", Digest(body), testSecret)
	headers := map[string]string{
		"Client-Id":         testClientID,
		"Request-Id":        "req-1",
		"Request-Timestamp": "2020-08-11T08:45:42Z",
		"Signature":         "HMACSHA256=" + signature,
	}

	// A changed body no longer matches the signature.
	err := client.VerifyNotification([]byte(`{"order":{"invoice_number":"SRB-1-1","amount":1}}`), headers)
	require.Error(t, err)
	assert.ErrorIs(t, err, payment.ErrInvalidSignature)

	// A notification signed by another merchant is rejected.
	wrongClient := map[string]string{
		"Client-Id":         "MCH-0002-00000000000000",
		"Request-Id":        "req-1",
		"Request-Timestamp": "2020-08-11T08:45:42Z",
		"Signature":         headers["Signature"],
	}
	err = client.VerifyNotification(body, wrongClient)
	require.Error(t, err)
	assert.ErrorIs(t, err, payment.ErrInvalidSignature)
}

func TestParseExpiredDate_UsesWIB(t *testing.T) {
	parsed := parseExpiredDate("20261024120000")

	require.False(t, parsed.IsZero())
	assert.Equal(t, 2026, parsed.Year())
	assert.Equal(t, time.October, parsed.Month())
	assert.Equal(t, 12, parsed.Hour())
}

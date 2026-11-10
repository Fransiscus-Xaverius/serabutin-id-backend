package doku

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

	assert.Equal(t, "dW7B055GxI7rWHtW4/fxKrR0WY51BNtzsH0CpN9/wJE=", signature)
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
		ClientID:  testClientID,
		SecretKey: testSecret,
		BaseURL:   server.URL,
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
		Channel:       "qris",
		RequestID:     "SRB-42-1",
	})

	require.NoError(t, err)
	assert.Equal(t, "https://checkout.doku.test/pay/1", result.PaymentURL)
	assert.Equal(t, "sess-1", result.ProviderSessionID)
	assert.Equal(t, "SRB-42-1", result.ProviderRequestID)
	assert.False(t, result.Replayed)

	// Signed request headers.
	assert.Equal(t, testClientID, capturedHeader.Get("Client-Id"))
	assert.Equal(t, "SRB-42-1", capturedHeader.Get("Request-Id"))
	assert.NotEmpty(t, capturedHeader.Get("Request-Timestamp"))
	assert.Contains(t, capturedHeader.Get("Signature"), "HMACSHA256=")

	// Body: QRIS only, correct amount, callback. Checkout settles into the
	// main merchant account, so no additional_info is sent.
	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &sent))

	order := sent["order"].(map[string]interface{})
	assert.Equal(t, float64(125000), order["amount"])
	assert.Equal(t, "SRB-42-1", order["invoice_number"])
	assert.Equal(t, "IDR", order["currency"])
	assert.Equal(t, "http://localhost:5173/payment/return?booking_id=42", order["callback_url_result"])
	assert.Equal(t, "http://localhost:5173/payment/return?booking_id=42", order["callback_url_cancel"])

	checkoutPayment := sent["payment"].(map[string]interface{})
	assert.Equal(t, []interface{}{"QRIS"}, checkoutPayment["payment_method_types"])
	assert.Equal(t, float64(60), checkoutPayment["payment_due_date"])

	_, hasAdditionalInfo := sent["additional_info"]
	assert.False(t, hasAdditionalInfo, "checkout must not route to a WaaS sub-account")

	customer := sent["customer"].(map[string]interface{})
	assert.Equal(t, "6281234567890", customer["phone"])
}

func TestCreateCheckout_OmitsAdditionalInfoWhenNotConfigured(t *testing.T) {
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
		Channel:       "qris",
		RequestID:     "SRB-43-1",
	})
	require.NoError(t, err)

	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &sent))
	_, hasAdditionalInfo := sent["additional_info"]
	assert.False(t, hasAdditionalInfo, "no additional_info without a notification override")
}

// With only the notification override configured, the additional_info object
// must carry the override and nothing else.
func TestCreateCheckout_OverrideOnlyConfigSendsNoAccount(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/6"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		ClientID:                testClientID,
		SecretKey:               testSecret,
		BaseURL:                 server.URL,
		OverrideNotificationURL: "https://tunnel.example/api/v1/payments/doku/webhook",
	})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		BookingID:     50,
		InvoiceNumber: "SRB-50-1",
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "qris",
		RequestID:     "SRB-50-1",
	})
	require.NoError(t, err)

	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &sent))

	additional, ok := sent["additional_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://tunnel.example/api/v1/payments/doku/webhook", additional["override_notification_url"])
	_, hasAccount := additional["account"]
	assert.False(t, hasAccount, "checkout has no sub-account routing")
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
		Channel:       "qris",
		RequestID:     "SRB-45-1",
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
		Channel:       "qris",
		RequestID:     "SRB-46-1",
	})
	require.NoError(t, err)

	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &sent))
	_, hasAdditional := sent["additional_info"]
	assert.False(t, hasAdditional)
}

func TestCreateCheckout_MapsChannelSlugToDOKUChannelName(t *testing.T) {
	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/5"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	// The slug is our vocabulary; DOKU only knows its own channel names.
	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-47-1",
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "ovo",
		RequestID:     "SRB-47-1",
	})
	require.NoError(t, err)

	var sent map[string]interface{}
	require.NoError(t, json.Unmarshal(capturedBody, &sent))
	checkoutPayment := sent["payment"].(map[string]interface{})
	assert.Equal(t, []interface{}{"EMONEY_OVO"}, checkoutPayment["payment_method_types"])
}

func TestCreateCheckout_RejectsUnknownChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/6"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-48-1",
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "carrier_pigeon",
		RequestID:     "SRB-48-1",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "carrier_pigeon")
}

func TestCreateCheckout_FailsWithoutCredentials(t *testing.T) {
	client := NewClient(Config{BaseURL: "https://api-sandbox.doku.com"})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-44-1",
		Amount:        10000,
		Channel:       "qris",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "DOKU_CLIENT_ID")
}

func TestCreateCheckout_UsesTheGivenRequestID(t *testing.T) {
	var capturedHeader http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Clone()
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/1"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-42-7",
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "qris",
		RequestID:     "SRB-42-7",
	})
	require.NoError(t, err)
	assert.Equal(t, "SRB-42-7", capturedHeader.Get("Request-Id"))
}

func TestCreateCheckout_RequiresARequestID(t *testing.T) {
	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: "https://api-sandbox.doku.com"})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-42-7",
		Amount:        10000,
		Channel:       "qris",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request id")
}

func TestCreateCheckout_Treats409WithTheOriginalBodyAsAReplay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/original"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	result, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-42-7",
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "qris",
		RequestID:     "SRB-42-7",
	})
	require.NoError(t, err, "a 409 carrying the original checkout is a replay, not a failure")
	assert.Equal(t, "https://checkout.doku.test/pay/original", result.PaymentURL)
	assert.True(t, result.Replayed)
}

func TestCreateCheckout_Treats409WithoutAResultAsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"something else"}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		InvoiceNumber: "SRB-42-7",
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "qris",
		RequestID:     "SRB-42-7",
	})
	require.Error(t, err)
}

func TestVerifyNotification_AcceptsMatchingSignature(t *testing.T) {
	body := []byte(`{"order":{"invoice_number":"SRB-1-1","amount":10000},"transaction":{"status":"SUCCESS"}}`)
	client := NewClient(Config{
		ClientID:         testClientID,
		SecretKey:        testSecret,
		BaseURL:          "https://api-sandbox.doku.com",
		NotificationPath: "/api/v1/payments/doku/webhook",
	})
	// Pin the clock to the notification timestamp so the replay guard does
	// not reject this 2020 test vector.
	client.now = func() time.Time { return time.Date(2020, 8, 11, 8, 45, 42, 0, time.UTC) }

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
	client.now = func() time.Time { return time.Date(2020, 8, 11, 8, 45, 42, 0, time.UTC) }

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

func TestVerifyNotification_RejectsStaleTimestamp(t *testing.T) {
	body := []byte(`{"order":{"invoice_number":"SRB-1-2","amount":10000},"transaction":{"status":"SUCCESS"}}`)
	client := NewClient(Config{
		ClientID:         testClientID,
		SecretKey:        testSecret,
		BaseURL:          "https://api-sandbox.doku.com",
		NotificationPath: "/api/v1/payments/doku/webhook",
	})
	// The notification is signed six minutes ago; the tolerance is 300
	// seconds, so a captured notification cannot be replayed later.
	client.now = func() time.Time { return time.Date(2020, 8, 11, 8, 51, 42, 0, time.UTC) }

	stale := "2020-08-11T08:45:42Z"
	signature := Sign(testClientID, "req-2", stale, "/api/v1/payments/doku/webhook", Digest(body), testSecret)

	err := client.VerifyNotification(body, map[string]string{
		"Client-Id":         testClientID,
		"Request-Id":        "req-2",
		"Request-Timestamp": stale,
		"Signature":         "HMACSHA256=" + signature,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, payment.ErrInvalidSignature)
}

func TestVerifyNotification_RejectsFutureTimestamp(t *testing.T) {
	body := []byte(`{"order":{"invoice_number":"SRB-1-3","amount":10000},"transaction":{"status":"SUCCESS"}}`)
	client := NewClient(Config{
		ClientID:         testClientID,
		SecretKey:        testSecret,
		BaseURL:          "https://api-sandbox.doku.com",
		NotificationPath: "/api/v1/payments/doku/webhook",
	})
	client.now = func() time.Time { return time.Date(2020, 8, 11, 8, 45, 42, 0, time.UTC) }

	future := "2020-08-11T08:51:42Z"
	signature := Sign(testClientID, "req-3", future, "/api/v1/payments/doku/webhook", Digest(body), testSecret)

	err := client.VerifyNotification(body, map[string]string{
		"Client-Id":         testClientID,
		"Request-Id":        "req-3",
		"Request-Timestamp": future,
		"Signature":         "HMACSHA256=" + signature,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, payment.ErrInvalidSignature)
}

func TestVerifyNotification_AcceptsOffsetTimestampWithinTolerance(t *testing.T) {
	body := []byte(`{"order":{"invoice_number":"SRB-1-4","amount":10000},"transaction":{"status":"SUCCESS"}}`)
	client := NewClient(Config{
		ClientID:         testClientID,
		SecretKey:        testSecret,
		BaseURL:          "https://api-sandbox.doku.com",
		NotificationPath: "/api/v1/payments/doku/webhook",
	})
	// 15:45:42+07:00 is the same instant as 08:45:42Z.
	client.now = func() time.Time { return time.Date(2020, 8, 11, 8, 45, 30, 0, time.UTC) }

	offset := "2020-08-11T15:45:42+07:00"
	signature := Sign(testClientID, "req-4", offset, "/api/v1/payments/doku/webhook", Digest(body), testSecret)

	err := client.VerifyNotification(body, map[string]string{
		"Client-Id":         testClientID,
		"Request-Id":        "req-4",
		"Request-Timestamp": offset,
		"Signature":         "HMACSHA256=" + signature,
	})

	require.NoError(t, err)
}

func TestCreateCheckout_RejectsOverlongInvoiceNumber(t *testing.T) {
	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: "https://api-sandbox.doku.com"})

	invoice := strings.Repeat("A", 65)
	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		BookingID:     48,
		InvoiceNumber: invoice,
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "qris",
		RequestID:     "SRB-48-1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "64")
}

func TestCreateCheckout_RejectsCardInvoiceOverThirtyCharacters(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"response":{"payment":{"url":"https://checkout.doku.test/pay/5"}}}`))
	}))
	defer server.Close()

	client := NewClient(Config{ClientID: testClientID, SecretKey: testSecret, BaseURL: server.URL})

	invoice := strings.Repeat("A", 31)
	_, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		BookingID:     49,
		InvoiceNumber: invoice,
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "card",
		RequestID:     "SRB-49-1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "30")
	assert.Equal(t, 0, calls, "an overlong card invoice must be rejected before any request leaves")

	// Exactly 30 characters passes the acquirer cap.
	ok, err := client.CreateCheckout(context.Background(), payment.CreateCheckoutRequest{
		BookingID:     49,
		InvoiceNumber: strings.Repeat("A", 30),
		Amount:        10000,
		ExpiryMinutes: 30,
		Channel:       "card",
		RequestID:     "SRB-49-2",
	})
	require.NoError(t, err)
	require.NotNil(t, ok)
	assert.Equal(t, 1, calls)
}

func TestParseExpiredDate_UsesWIB(t *testing.T) {
	parsed := parseExpiredDate("20261024120000")

	require.False(t, parsed.IsZero())
	assert.Equal(t, 2026, parsed.Year())
	assert.Equal(t, time.October, parsed.Month())
	assert.Equal(t, 12, parsed.Hour())
}

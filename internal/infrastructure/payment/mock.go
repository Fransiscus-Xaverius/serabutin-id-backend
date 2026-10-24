package payment

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// MockGateway produces a local simulation URL instead of a real DOKU
// Checkout session. It is only selected by DOKU_MODE=mock.
type MockGateway struct {
	publicURL string
	now       func() time.Time
}

func NewMockGateway(publicURL string) *MockGateway {
	return &MockGateway{publicURL: strings.TrimRight(publicURL, "/"), now: time.Now}
}

func (g *MockGateway) Name() string {
	return ProviderMock
}

func (g *MockGateway) CreateCheckout(_ context.Context, req CreateCheckoutRequest) (*CreateCheckoutResult, error) {
	expiry := time.Duration(req.ExpiryMinutes) * time.Minute
	if expiry <= 0 {
		expiry = 60 * time.Minute
	}
	bookingID := fmt.Sprintf("%d", req.BookingID)
	paymentURL := fmt.Sprintf("%s/payment/mock?invoice=%s&booking_id=%s",
		g.publicURL, url.QueryEscape(req.InvoiceNumber), url.QueryEscape(bookingID))

	return &CreateCheckoutResult{
		PaymentURL:        paymentURL,
		ProviderRequestID: "mock-" + req.InvoiceNumber,
		ProviderSessionID: "mock-" + req.InvoiceNumber,
		ExpiresAt:         g.now().Add(expiry),
	}, nil
}

// VerifyNotification is unused in mock mode: the mock succeed/expire
// endpoints apply status through the same usecase path without signatures.
func (g *MockGateway) VerifyNotification([]byte, map[string]string) error {
	return nil
}

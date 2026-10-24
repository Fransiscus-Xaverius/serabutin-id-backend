package payment

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	// ProviderDOKU and ProviderMock are the provider names stored on the
	// payment row. They mirror the entity constants so the infrastructure
	// layer does not need to import the domain package.
	ProviderDOKU = "doku"
	ProviderMock = "mock"
)

var (
	// ErrDisabled is returned when DOKU_MODE=disabled. Callers still record
	// a failed payment row so the customer sees a real failure.
	ErrDisabled = errors.New("payment gateway disabled")
	// ErrInvalidSignature is returned by notification verification and maps
	// to HTTP 401 on the public webhook.
	ErrInvalidSignature = errors.New("invalid payment notification signature")
)

type CreateCheckoutRequest struct {
	BookingID     uint
	CustomerID    uint
	InvoiceNumber string
	Amount        int64
	Currency      string
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
	CallbackURL   string
	ExpiryMinutes int
}

type CreateCheckoutResult struct {
	PaymentURL        string
	ProviderRequestID string
	ProviderSessionID string
	ExpiresAt         time.Time
}

// Gateway is the provider boundary used by the payment usecase. The DOKU
// implementation signs real Checkout requests; MockGateway only exists for
// local development.
type Gateway interface {
	Name() string
	CreateCheckout(ctx context.Context, req CreateCheckoutRequest) (*CreateCheckoutResult, error)
	VerifyNotification(rawBody []byte, headers map[string]string) error
}

type DisabledGateway struct{}

func NewDisabledGateway() *DisabledGateway {
	return &DisabledGateway{}
}

func (g *DisabledGateway) Name() string {
	return "doku"
}

func (g *DisabledGateway) CreateCheckout(context.Context, CreateCheckoutRequest) (*CreateCheckoutResult, error) {
	return nil, fmt.Errorf("%w: DOKU_MODE=disabled, cannot create a QRIS checkout", ErrDisabled)
}

func (g *DisabledGateway) VerifyNotification([]byte, map[string]string) error {
	return fmt.Errorf("%w: payment gateway disabled", ErrInvalidSignature)
}

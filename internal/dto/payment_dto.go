package dto

type PaymentResponse struct {
	Status        string  `json:"status"`
	Channel       string  `json:"channel"`
	InvoiceNumber string  `json:"invoice_number"`
	PaymentURL    string  `json:"payment_url"`
	ExpiresAt     *string `json:"expires_at,omitempty"`
	PaidAt        *string `json:"paid_at,omitempty"`
	Amount        int64   `json:"amount"`
}

type BookingConfigResponse struct {
	PlatformFee          int64    `json:"platform_fee"`
	PaymentExpiryMinutes int      `json:"payment_expiry_minutes"`
	PaymentMethods       []string `json:"payment_methods"`
}

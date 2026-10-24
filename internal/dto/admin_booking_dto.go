package dto

// AdminBookingRow is one ops-queue row: triage needs identity, schedule,
// status, and total — not the full booking payload.
type AdminBookingRow struct {
	ID           uint    `json:"id"`
	CustomerID   uint    `json:"customer_id"`
	ProviderID   uint    `json:"provider_id"`
	ServiceID    uint    `json:"service_id"`
	ScheduleDate *string `json:"schedule_date,omitempty"`
	Status       string  `json:"status"`
	Total        int64   `json:"total"`
	// PaymentMethod and PaymentStatus let ops see whether the money was
	// actually collected (and whether a refund is owed) without opening
	// every booking.
	PaymentMethod string `json:"payment_method,omitempty"`
	PaymentStatus string `json:"payment_status,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type AdminBookingListResponse struct {
	Data       []AdminBookingRow `json:"data"`
	Pagination PaginationMeta    `json:"pagination"`
}

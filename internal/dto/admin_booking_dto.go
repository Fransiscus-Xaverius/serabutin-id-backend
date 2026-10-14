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
	CreatedAt    string  `json:"created_at"`
}

type AdminBookingListResponse struct {
	Data       []AdminBookingRow `json:"data"`
	Pagination PaginationMeta    `json:"pagination"`
}

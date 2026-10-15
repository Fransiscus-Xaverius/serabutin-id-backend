package dto

// AdminBroadcastCreateRequest is the compose body. scheduled_at is optional:
// omit it for an instant blast, or pass a future RFC 3339 timestamp to
// schedule. No `reason` field — title/body are the auditable content and
// the admin audit middleware records the action automatically.
type AdminBroadcastCreateRequest struct {
	Title       string  `json:"title"`
	Body        string  `json:"body"`
	Audience    string  `json:"audience"`
	ScheduledAt *string `json:"scheduled_at"`
}

// AdminBroadcastResponse is one broadcast history row.
type AdminBroadcastResponse struct {
	ID             uint    `json:"id"`
	Title          string  `json:"title"`
	Body           string  `json:"body"`
	Audience       string  `json:"audience"`
	Status         string  `json:"status"`
	ScheduledAt    *string `json:"scheduled_at"`
	RecipientCount int64   `json:"recipient_count"`
	SentAt         *string `json:"sent_at"`
	CreatedBy      uint    `json:"created_by"`
	CreatedAt      string  `json:"created_at"`
}

type AdminBroadcastListResponse struct {
	Data       []AdminBroadcastResponse `json:"data"`
	Pagination PaginationMeta           `json:"pagination"`
}

// AdminBroadcastEstimateResponse backs the compose confirm modal.
type AdminBroadcastEstimateResponse struct {
	Audience       string `json:"audience"`
	RecipientCount int64  `json:"recipient_count"`
}

package dto

// AdminReviewRow is one moderation-queue row: identity, rating, comment
// preview, and the hidden flag — not the full review payload.
type AdminReviewRow struct {
	ID         uint   `json:"id"`
	BookingID  uint   `json:"booking_id"`
	ServiceID  uint   `json:"service_id"`
	CustomerID uint   `json:"customer_id"`
	ProviderID uint   `json:"provider_id"`
	AuthorID   uint   `json:"author_id"`
	AuthorRole string `json:"author_role"`
	Rating     int    `json:"rating"`
	Comment    string `json:"comment"`
	Hidden     bool   `json:"hidden"`
	CreatedAt  string `json:"created_at"`
}

type AdminReviewListResponse struct {
	Data       []AdminReviewRow `json:"data"`
	Pagination PaginationMeta   `json:"pagination"`
}

// AdminReviewVisibilityRequest carries the hide/keep flag plus its mandatory
// justification. The admin audit middleware persists `reason` automatically.
type AdminReviewVisibilityRequest struct {
	Hidden bool   `json:"hidden"`
	Reason string `json:"reason"`
}

// AdminReportRow is one report-queue row: who flagged what, why, and the
// lifecycle status.
type AdminReportRow struct {
	ID         uint    `json:"id"`
	ReporterID uint    `json:"reporter_id"`
	TargetType string  `json:"target_type"`
	TargetID   uint    `json:"target_id"`
	Reason     string  `json:"reason"`
	Detail     string  `json:"detail,omitempty"`
	Status     string  `json:"status"`
	ResolvedBy *uint   `json:"resolved_by,omitempty"`
	CreatedAt  string  `json:"created_at"`
}

type AdminReportListResponse struct {
	Data       []AdminReportRow `json:"data"`
	Pagination PaginationMeta   `json:"pagination"`
}

// AdminReportResolveRequest carries the terminal status plus its mandatory
// justification. The admin audit middleware persists `reason` automatically.
type AdminReportResolveRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

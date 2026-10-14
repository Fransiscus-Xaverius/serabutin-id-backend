package dto

// AdminServiceRow is one moderation-queue row: triage needs identity,
// pricing, and status — not the full service payload.
type AdminServiceRow struct {
	ID         uint   `json:"id"`
	ProviderID uint   `json:"provider_id"`
	Title      string `json:"title"`
	Category   string `json:"category"`
	BasePrice  int64  `json:"base_price"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

type AdminServiceListResponse struct {
	Data       []AdminServiceRow `json:"data"`
	Pagination PaginationMeta    `json:"pagination"`
}

// AdminServiceStatusRequest carries a moderation target plus its mandatory
// justification. The admin audit middleware persists `reason` automatically.
type AdminServiceStatusRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// AdminServiceProvider is the minimal provider context on the detail view.
type AdminServiceProvider struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// AdminServiceDetailResponse is the full inspectable listing: public service
// shape plus provider context. Image URLs are resolved like the public
// endpoint; moderation history is read from the audit log (no new columns).
type AdminServiceDetailResponse struct {
	ID              uint                   `json:"id"`
	ProviderID      uint                   `json:"provider_id"`
	Provider        *AdminServiceProvider  `json:"provider,omitempty"`
	Title           string                 `json:"title"`
	Category        string                 `json:"category"`
	Description     string                 `json:"description"`
	UnitType        string                 `json:"unit_type"`
	BasePrice       int64                  `json:"base_price"`
	BudgetMin       *int64                 `json:"budget_min,omitempty"`
	BudgetMax       *int64                 `json:"budget_max,omitempty"`
	AutoAcceptPrice *int64                 `json:"auto_accept_price,omitempty"`
	DurationMinutes int                    `json:"duration_minutes"`
	Images          []ServiceImageResponse `json:"images"`
	AddOns          []ServiceAddOnResponse `json:"add_ons"`
	ServiceAreas    string                 `json:"service_areas"`
	ServiceType     string                 `json:"service_type"`
	Status          string                 `json:"status"`
	CreatedAt       string                 `json:"created_at"`
}

package dto

type CreateBidRequest struct {
	ServiceID uint   `json:"service_id" validate:"required"`
	Price     int64  `json:"price" validate:"required,min=10000"`
	Notes     string `json:"notes,omitempty" validate:"omitempty,max=1000"`
}

type BidResponse struct {
	ID           uint              `json:"id"`
	ServiceID    uint              `json:"service_id"`
	Provider     *BookingUserInfo  `json:"provider,omitempty"`
	Price        int64             `json:"price"`
	Notes        string            `json:"notes,omitempty"`
	Status       string            `json:"status"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
	ServiceTitle string            `json:"service_title,omitempty"`
}

type BidListResponse struct {
	Data       []BidResponse  `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
}

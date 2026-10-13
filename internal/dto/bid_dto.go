package dto

type CreateBidRequest struct {
	ServiceID uint   `json:"service_id" validate:"required"`
	Price     int64  `json:"price" validate:"required,min=10000"`
	Notes     string `json:"notes,omitempty" validate:"omitempty,max=1000"`
}

type BidResponse struct {
	ID                 uint             `json:"id"`
	ServiceID          uint             `json:"service_id"`
	Provider           *BookingUserInfo `json:"provider,omitempty"`
	Price              int64            `json:"price"`
	Notes              string           `json:"notes,omitempty"`
	Status             string           `json:"status"`
	CreatedAt          string           `json:"created_at"`
	UpdatedAt          string           `json:"updated_at"`
	ServiceTitle       string           `json:"service_title,omitempty"`
	ServiceCategory    string           `json:"service_category,omitempty"`
	ServiceDescription string           `json:"service_description,omitempty"`
	ServiceLocation    string           `json:"service_location,omitempty"`
	ServiceImage       string           `json:"service_image,omitempty"`
	ServiceType        string           `json:"service_type,omitempty"`
	Customer           *BookingUserInfo `json:"customer,omitempty"`
}

type BidStatusCounts struct {
	Pending  int64 `json:"pending"`
	Accepted int64 `json:"accepted"`
	Rejected int64 `json:"rejected"`
}

type BidListResponse struct {
	Data       []BidResponse  `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
	Counts     BidStatusCounts `json:"counts"`
}

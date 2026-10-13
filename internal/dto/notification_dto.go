package dto

type NotificationResponse struct {
	ID        uint    `json:"id"`
	UserID    uint    `json:"user_id"`
	Type      string  `json:"type"`
	Title     string  `json:"title"`
	Body      string  `json:"body,omitempty"`
	Data      string  `json:"data,omitempty"`
	ReadAt    *string `json:"read_at,omitempty"`
	CreatedAt string  `json:"created_at"`
}

type NotificationListResponse struct {
	Data       []NotificationResponse `json:"data"`
	Pagination PaginationMeta         `json:"pagination"`
}

package dto

type CreateReviewRequest struct {
	BookingID           uint   `json:"booking_id" validate:"required"`
	Rating              int    `json:"rating" validate:"required,min=1,max=5"`
	CommunicationRating int    `json:"communication_rating" validate:"required,min=1,max=5"`
	QualityRating       int    `json:"quality_rating" validate:"required,min=1,max=5"`
	ValueRating         int    `json:"value_rating" validate:"required,min=1,max=5"`
	Comment             string `json:"comment" validate:"omitempty,max=1000"`
}

type ReviewResponse struct {
	ID                  uint           `json:"id"`
	Rating              int            `json:"rating"`
	CommunicationRating int            `json:"communication_rating"`
	QualityRating       int            `json:"quality_rating"`
	ValueRating         int            `json:"value_rating"`
	Comment             string         `json:"comment"`
	Customer            ReviewUserInfo `json:"customer"`
	CreatedAt           string         `json:"created_at"`
}

type ReviewUserInfo struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type ServiceReviewStatsResponse struct {
	TotalRating          float64 `json:"total_rating"`
	TotalCount           int64   `json:"total_count"`
	AverageCommunication float64 `json:"average_communication"`
	AverageQuality       float64 `json:"average_quality"`
	AverageValue         float64 `json:"average_value"`
}

type ReviewListResponse struct {
	Data       []ReviewResponse `json:"data"`
	Pagination PaginationMeta   `json:"pagination"`
}

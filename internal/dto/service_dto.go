package dto

type CreateServiceRequest struct {
	Title           string               `json:"title" validate:"required,min=3,max=100"`
	Category        string               `json:"category" validate:"required,oneof=cleaning handyman massage gardening ac_service moving laundry shopping errands other"`
	Description     string               `json:"description" validate:"required,min=10,max=2000"`
	UnitType        string               `json:"unit_type" validate:"required,oneof=per_session per_hour"`
	BasePrice       int64                `json:"base_price" validate:"required,min=10000"`
	DurationMinutes int                  `json:"duration_minutes" validate:"required,min=15"`
	Images          []ServiceImageRequest `json:"images" validate:"required,min=1,max=8,dive"`
	AddOns          []ServiceAddOnRequest `json:"add_ons,omitempty" validate:"omitempty,max=10,dive"`
	ServiceAreas    string               `json:"service_areas" validate:"required,max=500"`
	ServiceType     string               `json:"service_type,omitempty" validate:"omitempty,oneof=offered requested"`
	Status          string               `json:"status,omitempty" validate:"omitempty,oneof=draft active inactive"`
}

type UpdateServiceRequest struct {
	Title           string               `json:"title,omitempty" validate:"omitempty,min=3,max=100"`
	Category        string               `json:"category,omitempty" validate:"omitempty,oneof=cleaning handyman massage gardening ac_service moving laundry shopping errands other"`
	Description     string               `json:"description,omitempty" validate:"omitempty,min=10,max=2000"`
	UnitType        string               `json:"unit_type,omitempty" validate:"omitempty,oneof=per_session per_hour"`
	BasePrice       int64                `json:"base_price,omitempty" validate:"omitempty,min=10000"`
	DurationMinutes int                  `json:"duration_minutes,omitempty" validate:"omitempty,min=15"`
	Images          []ServiceImageRequest `json:"images,omitempty" validate:"omitempty,max=8,dive"`
	AddOns          []ServiceAddOnRequest `json:"add_ons,omitempty" validate:"omitempty,max=10,dive"`
	ServiceAreas    string               `json:"service_areas,omitempty" validate:"omitempty,max=500"`
	ServiceType     string               `json:"service_type,omitempty" validate:"omitempty,oneof=offered requested"`
	Status          string               `json:"status,omitempty" validate:"omitempty,oneof=draft active inactive"`
}

type ServiceImageRequest struct {
	URL       string `json:"url" validate:"required"`
	SortOrder int    `json:"sort_order"`
}

type ServiceAddOnRequest struct {
	Name  string `json:"name" validate:"required,min=1,max=100"`
	Price int64  `json:"price" validate:"required,min=0"`
}

type ServiceResponse struct {
	ID              uint                    `json:"id"`
	Provider        *ProviderInfo           `json:"provider,omitempty"`
	Title           string                  `json:"title"`
	Category        string                  `json:"category"`
	Description     string                  `json:"description"`
	UnitType        string                  `json:"unit_type"`
	BasePrice       int64                   `json:"base_price"`
	DurationMinutes int                     `json:"duration_minutes"`
	Images          []ServiceImageResponse  `json:"images"`
	AddOns          []ServiceAddOnResponse  `json:"add_ons"`
	ServiceAreas    string                  `json:"service_areas"`
	ServiceType     string                  `json:"service_type"`
	Status          string                  `json:"status"`
	Rating          float64                 `json:"rating"`
	ReviewCount     int64                   `json:"review_count"`
	CreatedAt       string                  `json:"created_at"`
	UpdatedAt       string                  `json:"updated_at,omitempty"`
}

type ServiceImageResponse struct {
	ID        uint   `json:"id"`
	URL       string `json:"url"`
	SortOrder int    `json:"sort_order"`
}

type ServiceAddOnResponse struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

type ProviderInfo struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url,omitempty"`
	City      string `json:"city,omitempty"`
	Province  string `json:"province,omitempty"`
}

type ServiceListResponse struct {
	Data       []ServiceResponse `json:"data"`
	Pagination PaginationMeta    `json:"pagination"`
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

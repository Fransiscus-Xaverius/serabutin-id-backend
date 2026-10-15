package dto

// Ticket 10: CMS (banners + categories) DTOs. Every admin mutation carries a
// mandatory `reason`, persisted to the audit log by the admin audit
// middleware; handlers enforce presence and this layer never writes audit
// rows directly.

type BannerResponse struct {
	ID        uint    `json:"id"`
	Title     string  `json:"title"`
	ImageURL  string  `json:"image_url"`
	LinkURL   string  `json:"link_url,omitempty"`
	Placement string  `json:"placement"`
	SortOrder int     `json:"sort_order"`
	Active    bool    `json:"active"`
	StartsAt  *string `json:"starts_at"`
	EndsAt    *string `json:"ends_at"`
	CreatedAt string  `json:"created_at"`
}

type BannerListResponse struct {
	Data []BannerResponse `json:"data"`
}

type AdminBannerCreateRequest struct {
	Title     string  `json:"title"`
	ImageURL  string  `json:"image_url"`
	LinkURL   string  `json:"link_url"`
	Placement string  `json:"placement"`
	SortOrder int     `json:"sort_order"`
	Active    *bool   `json:"active"`
	StartsAt  *string `json:"starts_at"`
	EndsAt    *string `json:"ends_at"`
	Reason    string  `json:"reason"`
}

type AdminBannerUpdateRequest struct {
	Title     *string `json:"title"`
	ImageURL  *string `json:"image_url"`
	LinkURL   *string `json:"link_url"`
	Placement *string `json:"placement"`
	SortOrder *int    `json:"sort_order"`
	Active    *bool   `json:"active"`
	StartsAt  *string `json:"starts_at"`
	EndsAt    *string `json:"ends_at"`
	Reason    string  `json:"reason"`
}

type CategoryResponse struct {
	ID        uint   `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Icon      string `json:"icon,omitempty"`
	SortOrder int    `json:"sort_order"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

type CategoryListResponse struct {
	Data []CategoryResponse `json:"data"`
}

type AdminCategoryCreateRequest struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	SortOrder int    `json:"sort_order"`
	Active    *bool  `json:"active"`
	Reason    string `json:"reason"`
}

type AdminCategoryUpdateRequest struct {
	Slug      *string `json:"slug"`
	Name      *string `json:"name"`
	Icon      *string `json:"icon"`
	SortOrder *int    `json:"sort_order"`
	Active    *bool   `json:"active"`
	Reason    string  `json:"reason"`
}

package entity

import "time"

// Banner is a CMS-managed home placement slot. The public storefront read
// only surfaces rows with Active=true whose schedule window contains now;
// the admin panel sees every status. Ticket 10.
type Banner struct {
	ID        uint
	Title     string
	ImageURL  string
	LinkURL   string
	Placement string
	SortOrder int
	Active    bool
	StartsAt  *time.Time
	EndsAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Category is one row of the service taxonomy. Service create/update
// validates Category against active rows (unknown slugs are rejected with
// 422 UNKNOWN_CATEGORY). Deactivating a row hides it publicly but never
// rewrites existing services. Ticket 10.
type Category struct {
	ID        uint
	Slug      string
	Name      string
	Icon      string
	SortOrder int
	Active    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

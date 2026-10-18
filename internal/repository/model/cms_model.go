package model

import "time"

// BannerModel is the CMS banner row. Placement defaults to "home"; the
// schedule window (starts_at/ends_at) is nullable — null means unbounded.
//
// `active` defaults to false: GORM substitutes the column default for a zero
// value on insert, so `default:true` would silently publish a banner the
// caller explicitly created as a draft (F1). An explicit `true` is still
// written as-is, which keeps the API-level default (active=true when the
// request omits the field) intact.
type BannerModel struct {
	ID        uint `gorm:"primaryKey"`
	Title     string `gorm:"size:255;not null"`
	ImageURL  string `gorm:"size:500;not null"`
	LinkURL   string `gorm:"size:500"`
	Placement string `gorm:"size:30;not null;default:home;index"`
	SortOrder int    `gorm:"not null;default:0"`
	Active    bool   `gorm:"not null;default:false;index"`
	StartsAt  *time.Time `gorm:"index"`
	EndsAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (BannerModel) TableName() string {
	return "banners"
}

// CategoryModel is one row of the service taxonomy. Slug is unique and is
// the value services store in services.category. `active` defaults to false
// for the same zero-value reason as BannerModel.Active (F1 class): a
// not-yet-reviewed row must not enter the public taxonomy by accident.
type CategoryModel struct {
	ID        uint `gorm:"primaryKey"`
	Slug      string `gorm:"size:50;uniqueIndex;not null"`
	Name      string `gorm:"size:100;not null"`
	Icon      string `gorm:"size:50"`
	SortOrder int    `gorm:"not null;default:0"`
	Active    bool   `gorm:"not null;default:false;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (CategoryModel) TableName() string {
	return "categories"
}

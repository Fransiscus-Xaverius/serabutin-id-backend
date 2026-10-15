package model

import "time"

// BannerModel is the CMS banner row. Placement defaults to "home"; the
// schedule window (starts_at/ends_at) is nullable — null means unbounded.
type BannerModel struct {
	ID        uint `gorm:"primaryKey"`
	Title     string `gorm:"size:255;not null"`
	ImageURL  string `gorm:"size:500;not null"`
	LinkURL   string `gorm:"size:500"`
	Placement string `gorm:"size:30;not null;default:home;index"`
	SortOrder int    `gorm:"not null;default:0"`
	Active    bool   `gorm:"not null;default:true;index"`
	StartsAt  *time.Time `gorm:"index"`
	EndsAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (BannerModel) TableName() string {
	return "banners"
}

// CategoryModel is one row of the service taxonomy. Slug is unique and is
// the value services store in services.category.
type CategoryModel struct {
	ID        uint `gorm:"primaryKey"`
	Slug      string `gorm:"size:50;uniqueIndex;not null"`
	Name      string `gorm:"size:100;not null"`
	Icon      string `gorm:"size:50"`
	SortOrder int    `gorm:"not null;default:0"`
	Active    bool   `gorm:"not null;default:true;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (CategoryModel) TableName() string {
	return "categories"
}

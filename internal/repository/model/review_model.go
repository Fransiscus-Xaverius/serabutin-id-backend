package model

import "time"

type ReviewModel struct {
	ID                  uint      `gorm:"primaryKey"`
	BookingID           uint      `gorm:"uniqueIndex:idx_reviews_booking_author,priority:1;not null"`
	ServiceID           uint      `gorm:"index;not null"`
	CustomerID          uint      `gorm:"not null"`
	ProviderID          uint      `gorm:"index;not null"`
	AuthorID            uint      `gorm:"uniqueIndex:idx_reviews_booking_author,priority:2;not null"`
	AuthorRole          string    `gorm:"size:20;not null;default:customer"`
	Rating              int       `gorm:"not null"`
	CommunicationRating int       `gorm:"not null"`
	QualityRating       int       `gorm:"not null"`
	ValueRating         int       `gorm:"not null"`
	Comment             string    `gorm:"size:1000"`
	// Hidden is set by admin moderation; public reads filter it out.
	Hidden              bool      `gorm:"not null;default:false;index"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (ReviewModel) TableName() string {
	return "reviews"
}

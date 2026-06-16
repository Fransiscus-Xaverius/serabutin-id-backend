package model

import "time"

type ReviewModel struct {
	ID                  uint      `gorm:"primaryKey"`
	BookingID           uint      `gorm:"uniqueIndex;not null"`
	ServiceID           uint      `gorm:"index;not null"`
	CustomerID          uint      `gorm:"not null"`
	ProviderID          uint      `gorm:"index;not null"`
	Rating              int       `gorm:"not null"`
	CommunicationRating int       `gorm:"not null"`
	QualityRating       int       `gorm:"not null"`
	ValueRating         int       `gorm:"not null"`
	Comment             string    `gorm:"size:1000"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (ReviewModel) TableName() string {
	return "reviews"
}

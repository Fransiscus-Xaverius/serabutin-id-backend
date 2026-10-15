package model

import "time"

type BroadcastModel struct {
	ID             uint `gorm:"primaryKey"`
	Title          string `gorm:"size:255;not null"`
	Body           string `gorm:"size:1000;not null"`
	Audience       string `gorm:"size:20;not null;index"`
	Status         string `gorm:"size:20;not null;index"`
	ScheduledAt    *time.Time `gorm:"index"`
	RecipientCount int64      `gorm:"not null;default:0"`
	SentAt         *time.Time
	CreatedBy      uint `gorm:"not null;index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (BroadcastModel) TableName() string {
	return "broadcasts"
}

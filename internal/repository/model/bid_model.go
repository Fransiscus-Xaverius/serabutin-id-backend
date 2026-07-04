package model

import (
	"time"

	"gorm.io/gorm"
)

type BidModel struct {
	ID         uint           `gorm:"primaryKey"`
	ServiceID  uint           `gorm:"index;not null"`
	ProviderID uint           `gorm:"index;not null"`
	Price      int64          `gorm:"not null"`
	Notes      string         `gorm:"size:1000"`
	Status     string         `gorm:"size:20;default:pending;not null"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

func (BidModel) TableName() string {
	return "bids"
}

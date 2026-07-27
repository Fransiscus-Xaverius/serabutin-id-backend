package model

import (
	"time"

	"gorm.io/gorm"
)

type UserModel struct {
	ID           uint           `gorm:"primaryKey"`
	Email        string         `gorm:"uniqueIndex;not null;size:255"`
	Password     string         `gorm:"not null"`
	Name         string         `gorm:"size:100;not null"`
	WhatsApp     string         `gorm:"uniqueIndex;not null;size:20"`
	Role         string         `gorm:"size:20;default:customer;not null"`
	Bio          string         `gorm:"size:500"`
	AvatarURL    string         `gorm:"size:500"`
	Address      string         `gorm:"size:255"`
	City         string         `gorm:"size:100"`
	Province     string         `gorm:"size:100"`
	Skills       string         `gorm:"size:500"`
	ServiceAreas string         `gorm:"size:500"`
	BudgetMin    *int64         `gorm:""`
	BudgetMax    *int64         `gorm:""`
	ConsentAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (UserModel) TableName() string {
	return "users"
}

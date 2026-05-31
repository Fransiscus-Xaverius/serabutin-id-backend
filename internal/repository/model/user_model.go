package model

import (
	"time"

	"gorm.io/gorm"
)

type UserModel struct {
	ID        uint           `gorm:"primaryKey"`
	Email     string         `gorm:"uniqueIndex;not null;size:255"`
	Password  string         `gorm:"not null"`
	Name      string         `gorm:"size:100;not null"`
	WhatsApp  string         `gorm:"uniqueIndex;not null;size:20"`
	Role      string         `gorm:"size:20;default:customer;not null"`
	ConsentAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (UserModel) TableName() string {
	return "users"
}

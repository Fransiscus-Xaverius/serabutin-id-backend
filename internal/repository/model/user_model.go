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
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (UserModel) TableName() string {
	return "users"
}

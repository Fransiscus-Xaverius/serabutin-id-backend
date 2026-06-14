package model

import (
	"time"

	"gorm.io/gorm"
)

type ServiceModel struct {
	ID              uint                `gorm:"primaryKey"`
	ProviderID      uint                `gorm:"index;not null"`
	Title           string              `gorm:"size:100;not null"`
	Category        string              `gorm:"size:50;index;not null"`
	Description     string              `gorm:"size:2000;not null"`
	UnitType        string              `gorm:"size:20;not null"`
	BasePrice       int64               `gorm:"not null"`
	DurationMinutes int                 `gorm:"not null"`
	ServiceAreas    string              `gorm:"size:500;not null"`
	Status          string              `gorm:"size:20;default:draft;index;not null"`
	Images          []ServiceImageModel `gorm:"foreignKey:ServiceID"`
	AddOns          []ServiceAddOnModel `gorm:"foreignKey:ServiceID"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`
}

func (ServiceModel) TableName() string {
	return "services"
}

type ServiceImageModel struct {
	ID        uint   `gorm:"primaryKey"`
	ServiceID uint   `gorm:"index;not null"`
	URL       string `gorm:"size:500;not null"`
	SortOrder int    `gorm:"default:0"`
}

func (ServiceImageModel) TableName() string {
	return "service_images"
}

type ServiceAddOnModel struct {
	ID        uint   `gorm:"primaryKey"`
	ServiceID uint   `gorm:"index;not null"`
	Name      string `gorm:"size:100;not null"`
	Price     int64  `gorm:"not null"`
}

func (ServiceAddOnModel) TableName() string {
	return "service_add_ons"
}

package model

import "time"

type BookingModel struct {
	ID             uint                `gorm:"primaryKey"`
	CustomerID     uint                `gorm:"index;not null"`
	ServiceID      uint                `gorm:"index;not null"`
	ProviderID     uint                `gorm:"index;not null"`
	ScheduleDate   *time.Time          `gorm:"null"`
	ScheduleTime   *string             `gorm:"size:5;null"`
	Address        *string             `gorm:"size:255;null"`
	Notes          string              `gorm:"size:500"`
	SubTotal       int64               `gorm:"not null"`
	PlatformFee    int64               `gorm:"not null"`
	Total          int64               `gorm:"not null"`
	PaymentMethod  string              `gorm:"size:30;not null"`
	Status         string              `gorm:"size:20;default:pending;index;not null"`
	BookingAddOns  []BookingAddOnModel `gorm:"foreignKey:BookingID"`
	Payment        *PaymentModel       `gorm:"foreignKey:BookingID"`
	CancellationID *uint
	CancelledBy    *uint
	CancelledAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (BookingModel) TableName() string {
	return "bookings"
}

type BookingAddOnModel struct {
	ID        uint   `gorm:"primaryKey"`
	BookingID uint   `gorm:"index;not null"`
	Name      string `gorm:"size:100;not null"`
	Price     int64  `gorm:"not null"`
}

func (BookingAddOnModel) TableName() string {
	return "booking_add_ons"
}

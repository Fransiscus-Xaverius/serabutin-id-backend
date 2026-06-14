package entity

import "time"

type Booking struct {
	ID            uint
	CustomerID    uint
	ServiceID     uint
	ProviderID    uint
	ScheduleDate  time.Time
	ScheduleTime  string
	Address       string
	Notes         string
	SubTotal      int64
	PlatformFee   int64
	Total         int64
	PaymentMethod string
	Status        string
	BookingAddOns []BookingAddOn
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type BookingAddOn struct {
	ID        uint
	BookingID uint
	Name      string
	Price     int64
}

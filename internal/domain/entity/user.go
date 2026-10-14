package entity

import "time"

const (
	UserStatusActive    = "active"
	UserStatusSuspended = "suspended"
)

type User struct {
	ID           uint
	Email        string
	Password     string
	Name         string
	WhatsApp     string
	Role         string
	Status       string
	Bio          string
	AvatarURL    string
	Address      string
	City         string
	Province     string
	Skills       string
	ServiceAreas string
	BudgetMin    *int64
	BudgetMax    *int64
	ConsentAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

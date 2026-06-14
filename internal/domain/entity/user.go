package entity

import "time"

type User struct {
	ID           uint
	Email        string
	Password     string
	Name         string
	WhatsApp     string
	Role         string
	Bio          string
	AvatarURL    string
	Address      string
	City         string
	Province     string
	Skills       string
	ServiceAreas string
	ConsentAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

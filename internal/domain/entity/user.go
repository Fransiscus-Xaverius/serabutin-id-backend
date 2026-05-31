package entity

import "time"

type User struct {
	ID        uint
	Email     string
	Password  string
	Name      string
	WhatsApp  string
	Role      string
	ConsentAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

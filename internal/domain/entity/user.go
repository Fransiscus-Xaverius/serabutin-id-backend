package entity

import "time"

type User struct {
	ID        uint
	Email     string
	Password  string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

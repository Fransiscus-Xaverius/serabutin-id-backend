package entity

import "time"

type Notification struct {
	ID        uint
	UserID    uint
	Type      string
	Title     string
	Body      string
	Data      string
	ReadAt    *time.Time
	CreatedAt time.Time
}

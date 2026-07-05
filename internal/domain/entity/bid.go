package entity

import "time"

type Bid struct {
	ID         uint
	ServiceID  uint
	ProviderID uint
	Price      int64
	Notes      string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

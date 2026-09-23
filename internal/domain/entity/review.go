package entity

import "time"

type Review struct {
	ID                  uint
	BookingID           uint
	ServiceID           uint
	CustomerID          uint
	ProviderID          uint
	AuthorID            uint
	AuthorRole          string
	Rating              int
	CommunicationRating int
	QualityRating       int
	ValueRating         int
	Comment             string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

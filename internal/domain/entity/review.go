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
	// Hidden marks a review concealed by admin moderation. Hidden rows are
	// excluded from every public read and aggregate; the row is retained.
	Hidden    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

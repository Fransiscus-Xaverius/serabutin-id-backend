package entity

import "time"

type Service struct {
	ID              uint
	ProviderID      uint
	Title           string
	Category        string
	Description     string
	UnitType        string
	BasePrice       int64
	BudgetMin       *int64
	BudgetMax       *int64
	AutoAcceptPrice *int64
	DurationMinutes int
	Images          []ServiceImage
	AddOns          []ServiceAddOn
	ServiceAreas    string
	Status          string
	ServiceType     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ServiceImage struct {
	ID        uint
	ServiceID uint
	URL       string
	SortOrder int
}

type ServiceAddOn struct {
	ID        uint
	ServiceID uint
	Name      string
	Price     int64
}

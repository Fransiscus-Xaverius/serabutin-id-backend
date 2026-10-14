package entity

import "time"

// Report is a user moderation flag against a platform target (service, user,
// review, or booking). Lifecycle: open -> resolved|dismissed (terminal).
type Report struct {
	ID         uint
	ReporterID uint
	TargetType string
	TargetID   uint
	Reason     string
	Detail     string
	Status     string
	ResolvedBy *uint
	ResolvedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Report target types.
const (
	ReportTargetService = "service"
	ReportTargetUser    = "user"
	ReportTargetReview  = "review"
	ReportTargetBooking = "booking"
)

// Report statuses.
const (
	ReportStatusOpen      = "open"
	ReportStatusResolved  = "resolved"
	ReportStatusDismissed = "dismissed"
)

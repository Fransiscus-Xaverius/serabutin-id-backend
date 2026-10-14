package entity

import "time"

// AuditLog is an attributable trail row for an admin mutation.
type AuditLog struct {
	ID          uint
	ActorUserID uint
	Action      string
	TargetType  string
	TargetID    string
	Reason      string
	CreatedAt   time.Time
}

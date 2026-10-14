package model

import "time"

// ReportModel persists a user moderation flag. Status lifecycle
// (open -> resolved|dismissed) is enforced by the admin usecase.
type ReportModel struct {
	ID         uint      `gorm:"primaryKey"`
	ReporterID uint      `gorm:"index;not null"`
	TargetType string    `gorm:"size:20;index;not null"`
	TargetID   uint      `gorm:"index;not null"`
	Reason     string    `gorm:"size:100;not null"`
	Detail     string    `gorm:"size:1000"`
	Status     string    `gorm:"size:20;index;not null;default:open"`
	ResolvedBy *uint     `gorm:"index"`
	ResolvedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (ReportModel) TableName() string {
	return "reports"
}

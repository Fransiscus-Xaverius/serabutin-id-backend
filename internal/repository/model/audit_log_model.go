package model

import "time"

type AuditLogModel struct {
	ID          uint      `gorm:"primaryKey"`
	ActorUserID uint      `gorm:"index:idx_audit_logs_actor_created,priority:1;not null"`
	Action      string    `gorm:"size:120;index;not null"`
	TargetType  string    `gorm:"size:40;index:idx_audit_logs_target,priority:1"`
	TargetID    string    `gorm:"size:40;index:idx_audit_logs_target,priority:2"`
	Reason      string    `gorm:"size:500"`
	CreatedAt   time.Time `gorm:"index:idx_audit_logs_actor_created,priority:2;index"`
}

func (AuditLogModel) TableName() string {
	return "audit_logs"
}

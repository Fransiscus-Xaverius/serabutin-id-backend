package model

import "time"

type NotificationModel struct {
	ID        uint       `gorm:"primaryKey"`
	UserID    uint       `gorm:"index:idx_notifications_user;not null"`
	Type      string     `gorm:"size:50;not null"`
	Title     string     `gorm:"size:255;not null"`
	Body      string     `gorm:"size:1000"`
	Data      string     `gorm:"type:text"`
	ReadAt    *time.Time `gorm:"index"`
	CreatedAt time.Time  `gorm:"index:idx_notifications_user"`
}

func (NotificationModel) TableName() string {
	return "notifications"
}

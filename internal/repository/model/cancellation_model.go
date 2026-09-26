package model

import "time"

type CancellationModel struct {
	ID                  uint      `gorm:"primaryKey"`
	BookingID           uint      `gorm:"uniqueIndex;not null"`
	CancelledBy         uint      `gorm:"index;not null"`
	CancelledByRole     string    `gorm:"size:20;not null"`
	Reason              string    `gorm:"size:30;not null"`
	Detail              string    `gorm:"size:500"`
	FeeCharged          int64     `gorm:"not null;default:0"`
	PlatformFeeSnapshot int64     `gorm:"not null;default:0"`
	SubtotalSnapshot    int64     `gorm:"not null;default:0"`
	PolicyVersion       string    `gorm:"size:10;not null;default:v1"`
	CreatedAt           time.Time `gorm:"index"`
}

func (CancellationModel) TableName() string {
	return "cancellations"
}

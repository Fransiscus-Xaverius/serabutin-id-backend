package model

import "time"

// ReimbursementModel persists a provider-filed money-dispute claim.
// Status lifecycle (pending -> approved|rejected) is enforced by the
// admin finance usecase; approval records the decision only.
type ReimbursementModel struct {
	ID          uint   `gorm:"primaryKey"`
	BookingID   uint   `gorm:"index;not null"`
	FiledBy     uint   `gorm:"index;not null"`
	Amount      int64  `gorm:"not null"`
	Reason      string `gorm:"size:500;not null"`
	EvidenceURL string `gorm:"size:500"`
	Status      string `gorm:"size:20;index;not null;default:pending"`
	DecidedBy   *uint  `gorm:"index"`
	DecidedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (ReimbursementModel) TableName() string {
	return "reimbursements"
}

// PayoutMarkModel persists an admin's manual "paid" mark for one provider
// period (unique per provider+period). No money movement is attached.
type PayoutMarkModel struct {
	ID         uint      `gorm:"primaryKey"`
	ProviderID uint      `gorm:"index;not null"`
	Period     string    `gorm:"size:7;not null;index"`
	MarkedBy   uint      `gorm:"not null"`
	MarkedAt   time.Time `gorm:"not null"`
	CreatedAt  time.Time
}

func (PayoutMarkModel) TableName() string {
	return "payout_marks"
}

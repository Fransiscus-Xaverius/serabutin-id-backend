package model

import "time"

type PayoutAccountModel struct {
	ID                         uint   `gorm:"primaryKey"`
	UserID                     uint   `gorm:"uniqueIndex;not null"`
	BankCode                   string `gorm:"size:16;not null"`
	BankName                   string `gorm:"size:100;not null"`
	AccountHolderNameEncrypted string `gorm:"type:text;not null"`
	AccountNumberEncrypted     string `gorm:"type:text;not null"`
	AccountLast4               string `gorm:"size:4;not null"`
	AccountFingerprint         string `gorm:"size:64;uniqueIndex"`
	EncryptionKeyID            string `gorm:"size:16;not null"`
	Status                     string `gorm:"size:20;not null;index"`
	NameMismatch               bool   `gorm:"not null;default:false"`
	ValidatedAt                *time.Time
	LastError                  string `gorm:"size:255"`
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

func (PayoutAccountModel) TableName() string { return "payout_accounts" }

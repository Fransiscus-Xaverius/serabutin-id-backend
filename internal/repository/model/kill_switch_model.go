package model

import "time"

// KillSwitchModel is the durable kill-switch intent row. Redis mirrors it
// at runtime (key killswitch:<module>); this table survives Redis flushes
// and is reloaded into Redis on boot.
type KillSwitchModel struct {
	Module    string    `gorm:"primaryKey;size:30"`
	Enabled   bool      `gorm:"not null;default:true"`
	UpdatedBy uint      `gorm:"not null;default:0"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	Reason    string    `gorm:"size:500"`
}

func (KillSwitchModel) TableName() string {
	return "kill_switches"
}

package entity

import "time"

// KillSwitch is the durable intent row for one emergency-brake module.
// The DB row is the source of truth for admin reads and boot reload;
// Redis mirrors it as the runtime source for hot-path reads.
type KillSwitch struct {
	Module    string
	Enabled   bool
	UpdatedBy uint
	UpdatedAt time.Time
	Reason    string
}

const (
	KillSwitchMaster       = "master"
	KillSwitchBookings     = "bookings"
	KillSwitchChat         = "chat"
	KillSwitchRegistration = "registration"
	KillSwitchBroadcasts   = "broadcasts"
)

// KillSwitchModules is the closed module set. `master` off freezes all four
// product modules (reads as OR: disabled if master off OR itself off).
var KillSwitchModules = []string{
	KillSwitchMaster,
	KillSwitchBookings,
	KillSwitchChat,
	KillSwitchRegistration,
	KillSwitchBroadcasts,
}

// IsValidKillSwitchModule reports whether module is a known kill-switch key.
func IsValidKillSwitchModule(module string) bool {
	for _, m := range KillSwitchModules {
		if m == module {
			return true
		}
	}
	return false
}

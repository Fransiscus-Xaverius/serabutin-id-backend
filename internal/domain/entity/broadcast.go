package entity

import "time"

// Broadcast audience values. Resolved to recipients at SEND time (not create
// time): all = every active user, customer|provider = active users in that
// role. Ticket 9, v1 delivery = in-app notifications only.
const (
	BroadcastAudienceAll      = "all"
	BroadcastAudienceCustomer = "customer"
	BroadcastAudienceProvider = "provider"
)

// Broadcast lifecycle: scheduled rows wait for the due-poller; instant rows
// are created as sending and flipped to sent after fan-out. Cancel is a
// scheduled -> cancelled CAS; sending -> scheduled requeue on delivery
// failure so the ticker retries instead of dropping the blast.
const (
	BroadcastStatusScheduled = "scheduled"
	BroadcastStatusSending   = "sending"
	BroadcastStatusSent      = "sent"
	BroadcastStatusCancelled = "cancelled"
)

type Broadcast struct {
	ID             uint
	Title          string
	Body           string
	Audience       string
	Status         string
	ScheduledAt    *time.Time
	RecipientCount int64
	SentAt         *time.Time
	CreatedBy      uint
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

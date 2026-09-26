package entity

import "time"

const CancellationPolicyVersion = "v1"

var ValidCancelReasons = map[string]bool{
	"changed_mind":      true,
	"found_other":       true,
	"schedule_conflict": true,
	"no_response":       true,
	"other":             true,
}

type Cancellation struct {
	ID                  uint
	BookingID           uint
	CancelledBy         uint
	CancelledByRole     string
	Reason              string
	Detail              string
	FeeCharged          int64
	PlatformFeeSnapshot int64
	SubtotalSnapshot    int64
	PolicyVersion       string
	CreatedAt           time.Time
}

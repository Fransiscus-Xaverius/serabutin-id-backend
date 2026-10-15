package middleware

import "testing"

// F4: audit target_type rendered "kill-switche" from a naive
// TrimSuffix(resource, "s") on "kill-switches".
func TestSingularizeAdminResource(t *testing.T) {
	cases := map[string]string{
		"kill-switches": "kill-switch",
		"audit-logs":    "audit-log",
		"users":         "user",
		"bookings":      "booking",
		"broadcasts":    "broadcast",
		"payouts":       "payout",
		"services":      "service",
		"reports":       "report",
		"reviews":       "review",
		"banners":       "banner",
	}
	for in, want := range cases {
		if got := singularizeAdminResource(in); got != want {
			t.Errorf("singularizeAdminResource(%q) = %q, want %q", in, got, want)
		}
	}
}

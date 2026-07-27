package usecase

import "testing"

// TDD guard for spec 002.auto-accept-penawaran-mitra.md.
// Pure function in auto_accept.go; tests assert the rule here.
func int64Ptr(v int64) *int64 { return &v }

func TestShouldAutoAccept(t *testing.T) {
	cases := []struct {
		name      string
		price     int64
		min, max  *int64
		want      bool
	}{
		{"diagram example: 10000 at lower bound", 10000, int64Ptr(10000), int64Ptr(20000), true},
		{"above upper bound: 25000", 25000, int64Ptr(10000), int64Ptr(20000), false},
		{"between min and max: 15000 (manual only)", 15000, int64Ptr(10000), int64Ptr(20000), false},
		{"at upper bound: 20000 (manual only)", 20000, int64Ptr(10000), int64Ptr(20000), false},
		{"below min: 5000 (auto-accept — cheap work)", 5000, int64Ptr(10000), int64Ptr(20000), true},
		{"unset min", 10000, nil, int64Ptr(20000), false},
		{"unset max", 10000, int64Ptr(10000), nil, false},
		{"both unset", 10000, nil, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shouldAutoAccept(c.price, c.min, c.max); got != c.want {
				t.Errorf("shouldAutoAccept(%d, min=%v, max=%v) = %v, want %v",
					c.price, c.min, c.max, got, c.want)
			}
		})
	}
}
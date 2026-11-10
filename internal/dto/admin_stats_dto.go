package dto

// AdminStatsUsersResponse is the users block of GET /admin/stats.
type AdminStatsUsersResponse struct {
	Total  int64            `json:"total"`
	ByRole map[string]int64 `json:"by_role"`
	New7d  int64            `json:"new_7d"`
}

// AdminStatsBookingsResponse is the bookings block of GET /admin/stats.
type AdminStatsBookingsResponse struct {
	ByStatus      map[string]int64 `json:"by_status"`
	Total30d      int64            `json:"total_30d"`
	CancelRate30d float64          `json:"cancel_rate_30d"`
}

// AdminStatsGMVResponse is the gmv block of GET /admin/stats.
type AdminStatsGMVResponse struct {
	Total30d        int64 `json:"total_30d"`
	PlatformFees30d int64 `json:"platform_fees_30d"`
}

// AdminStatsServicesResponse is the services block of GET /admin/stats.
type AdminStatsServicesResponse struct {
	PendingCount int64 `json:"pending_count"`
	ActiveCount  int64 `json:"active_count"`
}

// AdminStatsReportsResponse is the reports block of GET /admin/stats.
type AdminStatsReportsResponse struct {
	OpenCount int64 `json:"open_count"`
}

// AdminStatsResponse is the full GET /admin/stats payload. Maps are always
// initialized (never null) so an empty platform yields zeros, not nulls.
type AdminStatsResponse struct {
	Users    AdminStatsUsersResponse    `json:"users"`
	Bookings AdminStatsBookingsResponse `json:"bookings"`
	GMV      AdminStatsGMVResponse      `json:"gmv"`
	Services AdminStatsServicesResponse `json:"services"`
	Reports  AdminStatsReportsResponse  `json:"reports"`
}

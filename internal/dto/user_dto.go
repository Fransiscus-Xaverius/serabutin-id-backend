package dto

type PublicUserResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

type UserStatsResponse struct {
	User           PublicUserResponse `json:"user"`
	Role           string             `json:"role"`
	MemberSince    string             `json:"member_since"`
	JobsPosted     int64              `json:"jobs_posted"`
	ActiveListings int64              `json:"active_listings"`
	HireRate       float64            `json:"hire_rate"`
	TotalSpent     int64              `json:"total_spent"`
}

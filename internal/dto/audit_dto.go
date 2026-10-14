package dto

type AuditLogResponse struct {
	ID          uint   `json:"id"`
	ActorUserID uint   `json:"actor_user_id"`
	Action      string `json:"action"`
	TargetType  string `json:"target_type"`
	TargetID    string `json:"target_id"`
	Reason      string `json:"reason"`
	CreatedAt   string `json:"created_at"`
}

type AuditLogListResponse struct {
	Data       []AuditLogResponse `json:"data"`
	Pagination PaginationMeta     `json:"pagination"`
}

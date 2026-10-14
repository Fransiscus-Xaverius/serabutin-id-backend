package dto

// AdminChatBlockRequest carries a chat-only block toggle plus its mandatory
// justification. Blocked is a pointer so a missing field is distinguishable
// from an explicit unblock (false); both still require a reason.
type AdminChatBlockRequest struct {
	Blocked *bool  `json:"blocked"`
	Reason  string `json:"reason"`
}

// AdminConversationRow is conversation METADATA only (ticket 8, v1 scope).
// Message bodies, previews, and content are never exposed here — the shape
// intentionally has no body/preview/content field.
type AdminConversationRow struct {
	ConversationID string `json:"conversation_id"`
	UserAID        uint   `json:"user_a_id"`
	UserBID        uint   `json:"user_b_id"`
	MessageCount   int64  `json:"message_count"`
	LastActivityAt string `json:"last_activity_at"`
	// Flags is the report count for this conversation. Always 0 in v1: the
	// reports table keys targets by uint while chat conversation IDs are
	// UUIDs, so no join is possible without a schema follow-up.
	Flags int64 `json:"flags"`
}

type AdminConversationListResponse struct {
	Data       []AdminConversationRow `json:"data"`
	Pagination PaginationMeta         `json:"pagination"`
}

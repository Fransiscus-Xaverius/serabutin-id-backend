package dto

type IntrospectRequest struct {
	Token string `json:"token"`
}

type IntrospectResponse struct {
	Active bool   `json:"active"`
	UserID uint   `json:"user_id,omitempty"`
	Email  string `json:"email,omitempty"`
}

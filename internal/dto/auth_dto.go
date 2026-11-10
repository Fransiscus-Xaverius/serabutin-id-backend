package dto

type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	Name     string `json:"name" validate:"required,min=2,max=100"`
	WhatsApp string `json:"whatsapp" validate:"required,whatsapp"`
	Role     string `json:"role" validate:"required,oneof=customer provider"`
}

type LoginRequest struct {
	Identifier string `json:"identifier" validate:"required"`
	Password   string `json:"password" validate:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token,omitempty"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	User         UserResponse `json:"user"`
}

type UserResponse struct {
	ID           uint   `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	WhatsApp     string `json:"whatsapp"`
	Role         string `json:"role"`
	Bio          string `json:"bio,omitempty"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	Address      string `json:"address,omitempty"`
	City         string `json:"city,omitempty"`
	Province     string `json:"province,omitempty"`
	Skills       string `json:"skills,omitempty"`
	ServiceAreas string `json:"service_areas,omitempty"`
	BudgetMin    *int64 `json:"budget_min,omitempty"`
	BudgetMax    *int64 `json:"budget_max,omitempty"`
	CreatedAt    string `json:"created_at"`
}

type IntrospectRequest struct {
	Token string `json:"token" validate:"required"`
}

type IntrospectResponse struct {
	Active       bool   `json:"active"`
	UserID       uint   `json:"user_id,omitempty"`
	Email        string `json:"email,omitempty"`
	ChatBlocked  bool   `json:"chat_blocked,omitempty"`
	ChatDisabled bool   `json:"chat_disabled,omitempty"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

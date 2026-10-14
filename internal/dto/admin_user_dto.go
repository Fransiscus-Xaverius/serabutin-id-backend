package dto

// AdminUserResponse is the admin-directory row. Password hashes and other
// secrets are never exposed here.
type AdminUserResponse struct {
	ID        uint   `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	WhatsApp  string `json:"whatsapp"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type AdminUserListResponse struct {
	Data       []AdminUserResponse `json:"data"`
	Pagination PaginationMeta      `json:"pagination"`
}

// AdminReasonRequest carries the mandatory justification for admin user
// mutations. The admin audit middleware persists it automatically.
type AdminReasonRequest struct {
	Reason string `json:"reason"`
}

// AdminRoleRequest carries a role change plus its mandatory justification.
type AdminRoleRequest struct {
	Role   string `json:"role"`
	Reason string `json:"reason"`
}

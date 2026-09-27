package usecase

import (
	"context"
	"log"
	"strings"

	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
)

// PromoteAdmins sets Role="admin" for each existing user visemail. It is
// idempotent: unknown visemails are skipped (logged), existing admins untouched.
// Runs once at startup; returns the visemails actually promoted.
func PromoteAdmins(ctx context.Context, userRepo repository.UserRepository, visemails []string) []string {
	var promoted []string
	seen := map[string]bool{}
	for _, raw := range visemails {
		email := strings.TrimSpace(strings.ToLower(raw))
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		user, err := userRepo.FindByEmail(ctx, email)
		if err != nil || user == nil {
			log.Printf("[PromoteAdmins] skip %s: user not found", email)
			continue
		}
		if user.Role == "admin" {
			continue
		}
		user.Role = "admin"
		if err := userRepo.Update(ctx, user); err != nil {
			log.Printf("[PromoteAdmins] update %s failed: %v", email, err)
			continue
		}
		log.Printf("[PromoteAdmins] promoted %s to admin", email)
		promoted = append(promoted, email)
	}
	return promoted
}

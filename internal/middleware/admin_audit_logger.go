package middleware

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// auditLogWriter is the minimal surface AdminAuditLogger needs — implemented
// by *gormrepo.AuditLogRepository. Kept as an interface so middleware does
// not import the repository package.
type auditLogWriter interface {
	Create(ctx context.Context, entry *entity.AuditLog) error
}

// AdminAuditLogger persists an audit row for every successful /admin/*
// mutation (POST/PUT/PATCH/DELETE). GETs are never audited, failed responses
// (status >= 400) are never audited, and persistence is best-effort: a write
// failure is logged but never fails the admin action.
func AdminAuditLogger(repo auditLogWriter, log zerolog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()

		if !isAdminMutation(c) {
			return err
		}
		if c.Response().StatusCode() >= 400 {
			return err
		}
		actor := requestctx.GetUserID(c)
		if actor == 0 {
			return err
		}

		entry := &entity.AuditLog{
			ActorUserID: actor,
			Action:      adminAction(c),
			TargetType:  adminTargetType(c),
			TargetID:    c.Params("id"),
			Reason:      adminReason(c),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if werr := repo.Create(ctx, entry); werr != nil {
			log.Error().Err(werr).
				Uint("actor_user_id", actor).
				Str("action", entry.Action).
				Str("request_id", requestctx.GetRequestID(c)).
				Msg("admin audit log write failed")
		}

		return err
	}
}

func isAdminMutation(c fiber.Ctx) bool {
	switch c.Method() {
	case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		return true
	default:
		return false
	}
}

// adminAction renders "<METHOD> <route pattern>", e.g.
// "PATCH /api/v1/admin/users/:id/suspend".
func adminAction(c fiber.Ctx) string {
	pattern := c.Path()
	if r := c.Route(); r != nil && r.Path != "" {
		pattern = r.Path
	}
	return c.Method() + " " + pattern
}

// adminTargetType derives the audited resource from the route pattern, e.g.
// "/api/v1/admin/users/:id/suspend" -> "user". Empty when underivable.
func adminTargetType(c fiber.Ctx) string {
	pattern := c.Path()
	if r := c.Route(); r != nil && r.Path != "" {
		pattern = r.Path
	}
	segments := strings.Split(strings.ToLower(pattern), "/")
	for i, seg := range segments {
		if seg == "admin" && i+1 < len(segments) {
			resource := segments[i+1]
			return strings.TrimSuffix(resource, "s")
		}
	}
	return ""
}

// adminReason extracts the `reason` field from a JSON body when present,
// falling back to the `reason` query param (DELETEs without a body).
func adminReason(c fiber.Ctx) string {
	if len(c.Body()) > 0 {
		var payload struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(c.Body(), &payload) == nil && payload.Reason != "" {
			return payload.Reason
		}
	}
	return c.Query("reason", "")
}

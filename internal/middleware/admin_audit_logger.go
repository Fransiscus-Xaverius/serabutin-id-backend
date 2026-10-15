package middleware

import (
	"context"
	"encoding/json"
	"strconv"
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
			TargetID:    adminTargetID(c),
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
			return singularizeAdminResource(resource)
		}
	}
	return ""
}

// singularizeAdminResource renders the singular audited resource
// ("users" -> "user"). F4: a naive TrimSuffix "s" mangles -ches/-shes
// plurals, so "kill-switches" became "kill-switche"; strip "es" after a
// sibilant ending first.
func singularizeAdminResource(resource string) string {
	for _, suffix := range []string{"ches", "shes", "sses", "xes", "zes"} {
		if strings.HasSuffix(resource, suffix) && len(resource) > len(suffix) {
			return resource[:len(resource)-2]
		}
	}
	return strings.TrimSuffix(resource, "s")
}

// adminTargetID resolves the audited target when the route carries no `:id`
// param. F4: kill-switch routes (`PATCH /admin/kill-switches/:module`) use
// the module name, and payout marks (`POST /admin/payouts/mark`, body-only)
// record `provider_id/period`. Without a fallback these mutations left empty
// TargetID rows.
func adminTargetID(c fiber.Ctx) string {
	if id := c.Params("id"); id != "" {
		return id
	}
	if module := c.Params("module"); module != "" {
		return module
	}
	return payoutMarkTarget(c)
}

// payoutMarkTarget derives `provider_id/period` from the mark body so the
// paid mark is attributable to the provider period it records. Empty unless
// both fields parse.
func payoutMarkTarget(c fiber.Ctx) string {
	if len(c.Body()) == 0 {
		return ""
	}
	var payload struct {
		ProviderID uint   `json:"provider_id"`
		Period     string `json:"period"`
	}
	if json.Unmarshal(c.Body(), &payload) != nil {
		return ""
	}
	if payload.ProviderID == 0 || strings.TrimSpace(payload.Period) == "" {
		return ""
	}
	return strconv.FormatUint(uint64(payload.ProviderID), 10) + "/" + strings.TrimSpace(payload.Period)
}

// adminReason extracts the `reason` field from a JSON body when present,
// falling back to the `reason` query param (DELETEs without a body). F4: on
// kill-switch routes the new toggle state is appended (`<reason>
// [enabled=false]`) so the audit row records what the switch was set to,
// not just that it was touched.
func adminReason(c fiber.Ctx) string {
	reason := ""
	if len(c.Body()) > 0 {
		var payload struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(c.Body(), &payload) == nil && payload.Reason != "" {
			reason = payload.Reason
		}
	}
	if reason == "" {
		reason = c.Query("reason", "")
	}
	if state := killSwitchState(c); state != "" && reason != "" {
		reason += " " + state
	} else if state != "" {
		reason = state
	}
	return reason
}

// killSwitchState renders the toggled state (`[enabled=false]`) from the
// kill-switch body. Empty on non-kill-switch routes or unparsable bodies.
func killSwitchState(c fiber.Ctx) string {
	if c.Params("module") == "" || len(c.Body()) == 0 {
		return ""
	}
	var payload struct {
		Enabled *bool `json:"enabled"`
	}
	if json.Unmarshal(c.Body(), &payload) != nil || payload.Enabled == nil {
		return ""
	}
	if *payload.Enabled {
		return "[enabled=true]"
	}
	return "[enabled=false]"
}

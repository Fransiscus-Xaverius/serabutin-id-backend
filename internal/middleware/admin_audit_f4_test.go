package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"io"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/serabutin/serabutin-id-backend/internal/repository/gorm"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// F4: kill-switch and payout-mark mutations must leave attributable audit
// rows instead of empty TargetIDs.

func adminAuditF4TestApp(db *gorm.DB) *fiber.App {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 7)
		return c.Next()
	})
	app.Use(AdminAuditLogger(gormrepo.NewAuditLogRepository(db), zerolog.New(io.Discard)))
	app.Patch("/api/v1/admin/kill-switches/:module", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
	})
	app.Post("/api/v1/admin/payouts/mark", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
	})
	return app
}

func lastAuditRow(t *testing.T, db *gorm.DB) model.AuditLogModel {
	t.Helper()
	var row model.AuditLogModel
	require.NoError(t, db.Order("id DESC").First(&row).Error)
	return row
}

func TestAdminAuditLogger_KillSwitch_RecordsModuleAndState(t *testing.T) {
	db := setupAdminAuditTestDB(t)
	app := adminAuditF4TestApp(db)

	req := httptest.NewRequest("PATCH", "/api/v1/admin/kill-switches/chat", strings.NewReader(`{"enabled":false,"reason":"insiden spam"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	row := lastAuditRow(t, db)
	assert.Equal(t, "chat", row.TargetID)
	assert.Contains(t, row.Reason, "insiden spam")
	assert.Contains(t, row.Reason, "[enabled=false]")
}

func TestAdminAuditLogger_PayoutMark_RecordsProviderPeriod(t *testing.T) {
	db := setupAdminAuditTestDB(t)
	app := adminAuditF4TestApp(db)

	req := httptest.NewRequest("POST", "/api/v1/admin/payouts/mark", strings.NewReader(`{"provider_id":12,"period":"2026-10","reason":"transfer manual"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	row := lastAuditRow(t, db)
	assert.Equal(t, "12/2026-10", row.TargetID)
	assert.Equal(t, "transfer manual", row.Reason)
}

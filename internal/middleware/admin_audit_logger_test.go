package middleware

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/serabutin/serabutin-id-backend/internal/repository/gorm"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

func setupAdminAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := testpostgres.Run(ctx,
		"postgres:16-alpine",
		testpostgres.WithDatabase("testdb"),
		testpostgres.WithUsername("test"),
		testpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := gorm.Open(gormpostgres.Open(connStr), &gorm.Config{TranslateError: true})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AuditLogModel{}))
	return db
}

func adminAuditTestApp(db *gorm.DB) *fiber.App {
	app := fiber.New()
	// Stub auth: every request acts as admin user 7.
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 7)
		return c.Next()
	})
	app.Use(AdminAuditLogger(gormrepo.NewAuditLogRepository(db), zerolog.New(io.Discard)))
	app.Patch("/api/v1/admin/users/:id/suspend", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
	})
	app.Patch("/api/v1/admin/users/:id/broken", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"ok": false})
	})
	app.Get("/api/v1/admin/audit-logs", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
	})
	return app
}

func countAuditRows(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&model.AuditLogModel{}).Count(&n).Error)
	return n
}

func TestAdminAuditLogger_MutationWritesAuditRow(t *testing.T) {
	db := setupAdminAuditTestDB(t)
	app := adminAuditTestApp(db)

	req := httptest.NewRequest("PATCH", "/api/v1/admin/users/42/suspend", strings.NewReader(`{"reason":"spam reports"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	require.Equal(t, int64(1), countAuditRows(t, db))
	var row model.AuditLogModel
	require.NoError(t, db.First(&row).Error)
	assert.Equal(t, uint(7), row.ActorUserID)
	assert.Contains(t, row.Action, "PATCH")
	assert.Contains(t, row.Action, "suspend")
	assert.Equal(t, "42", row.TargetID)
	assert.Equal(t, "spam reports", row.Reason)
	assert.False(t, row.CreatedAt.IsZero())
}

func TestAdminAuditLogger_SkipsGETs(t *testing.T) {
	db := setupAdminAuditTestDB(t)
	app := adminAuditTestApp(db)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit-logs", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, int64(0), countAuditRows(t, db))
}

func TestAdminAuditLogger_SkipsFailedMutations(t *testing.T) {
	db := setupAdminAuditTestDB(t)
	app := adminAuditTestApp(db)

	req := httptest.NewRequest("PATCH", "/api/v1/admin/users/42/broken", strings.NewReader(`{"reason":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, int64(0), countAuditRows(t, db))
}

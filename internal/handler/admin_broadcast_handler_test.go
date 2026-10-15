package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

// Ticket 9: admin broadcast endpoints — compose (instant/scheduled),
// history, estimate, cancel. Mandatory title/body/audience.

type stubBroadcastRepo struct {
	created    *entity.Broadcast
	createErr  error
	rows       []*entity.Broadcast
	total      int64
	listErr    error
	cancelled  *entity.Broadcast
	cancelErr  error
	count      int64
	countErr   error
	claimed    []*entity.Broadcast
	sent       map[uint]int64
	notifTypes []string
}

func (s *stubBroadcastRepo) Create(_ context.Context, b *entity.Broadcast) error {
	if s.createErr != nil {
		return s.createErr
	}
	b.ID = 5
	b.CreatedAt = time.Now()
	s.created = b
	return nil
}

func (s *stubBroadcastRepo) FindByID(_ context.Context, id uint) (*entity.Broadcast, error) {
	return &entity.Broadcast{ID: id}, nil
}

func (s *stubBroadcastRepo) List(_ context.Context, _, _ int) ([]*entity.Broadcast, int64, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.rows, s.total, nil
}

func (s *stubBroadcastRepo) Cancel(_ context.Context, _ uint) (*entity.Broadcast, error) {
	if s.cancelErr != nil {
		return nil, s.cancelErr
	}
	return s.cancelled, nil
}

func (s *stubBroadcastRepo) ClaimDue(_ context.Context, _ time.Time, _ int) ([]*entity.Broadcast, error) {
	return s.claimed, nil
}

func (s *stubBroadcastRepo) MarkSent(_ context.Context, id uint, count int64) (*entity.Broadcast, error) {
	if s.sent == nil {
		s.sent = map[uint]int64{}
	}
	s.sent[id] = count
	now := time.Now()
	return &entity.Broadcast{
		ID: id, Title: s.created.Title, Body: s.created.Body, Audience: s.created.Audience,
		Status: entity.BroadcastStatusSent, RecipientCount: count, SentAt: &now, CreatedBy: 7,
	}, nil
}

func (s *stubBroadcastRepo) Requeue(_ context.Context, _ uint) error { return nil }

func (s *stubBroadcastRepo) CountRecipients(_ context.Context, _ string) (int64, error) {
	if s.countErr != nil {
		return 0, s.countErr
	}
	return s.count, nil
}

func (s *stubBroadcastRepo) ListRecipientIDs(_ context.Context, audience string, _ uint, _ int) ([]uint, error) {
	if audience == "customer" {
		return []uint{10, 11}, nil
	}
	return nil, nil
}

type stubBroadcastNotifier struct {
	rows []*entity.Notification
}

func (s *stubBroadcastNotifier) Create(_ context.Context, _ *entity.Notification) error {
	return nil
}

func (s *stubBroadcastNotifier) CreateMany(_ context.Context, rows []*entity.Notification) error {
	s.rows = append(s.rows, rows...)
	return nil
}

func (s *stubBroadcastNotifier) ListByUserID(_ context.Context, _ uint, _, _ int) ([]*entity.Notification, int64, error) {
	return nil, 0, nil
}

func (s *stubBroadcastNotifier) ListSinceID(_ context.Context, _ uint, _ uint, _ int) ([]*entity.Notification, error) {
	return nil, nil
}

func (s *stubBroadcastNotifier) ListAll(_ context.Context, _, _ int, _ string) ([]*entity.Notification, int64, error) {
	return nil, 0, nil
}

func (s *stubBroadcastNotifier) MarkRead(_ context.Context, _ uint, _ uint) error { return nil }

func newAdminBroadcastTestApp(t *testing.T, repo *stubBroadcastRepo, notifier *stubBroadcastNotifier) *fiber.App {
	t.Helper()
	uc := usecase.NewAdminBroadcastUsecase(repo, notifier)
	h := NewAdminHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetAdminBroadcastUC(uc)
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		requestctx.SetUserID(c, 7)
		return c.Next()
	})
	app.Get("/api/v1/admin/broadcasts", h.ListBroadcasts)
	app.Get("/api/v1/admin/broadcasts/estimate", h.EstimateBroadcastRecipients)
	app.Post("/api/v1/admin/broadcasts", h.CreateBroadcast)
	app.Post("/api/v1/admin/broadcasts/:id/cancel", h.CancelBroadcast)
	return app
}

func doBroadcastRequest(t *testing.T, app *fiber.App, method, url, body string) (int, map[string]interface{}) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return resp.StatusCode, decoded
}

func TestAdminHandler_CreateBroadcast_InstantSendsAndReturns201(t *testing.T) {
	t.Parallel()
	repo := &stubBroadcastRepo{}
	notifier := &stubBroadcastNotifier{}
	app := newAdminBroadcastTestApp(t, repo, notifier)

	status, body := doBroadcastRequest(t, app, "POST", "/api/v1/admin/broadcasts",
		`{"title":"Promo","body":"Diskon 20%.","audience":"customer"}`)

	assert.Equal(t, fiber.StatusCreated, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "sent", data["status"])
	assert.Equal(t, float64(2), data["recipient_count"])
	require.Len(t, notifier.rows, 2)
	for _, n := range notifier.rows {
		assert.Equal(t, "admin.broadcast", n.Type)
	}
}

func TestAdminHandler_CreateBroadcast_ScheduledStoresWithoutSending(t *testing.T) {
	t.Parallel()
	repo := &stubBroadcastRepo{}
	notifier := &stubBroadcastNotifier{}
	app := newAdminBroadcastTestApp(t, repo, notifier)
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	status, body := doBroadcastRequest(t, app, "POST", "/api/v1/admin/broadcasts",
		`{"title":"Maintenance","body":"Down 00-02.","audience":"provider","scheduled_at":"`+future+`"}`)

	assert.Equal(t, fiber.StatusCreated, status)
	assert.Equal(t, "scheduled", body["data"].(map[string]interface{})["status"])
	assert.Empty(t, notifier.rows)
}

func TestAdminHandler_CreateBroadcast_MissingFields_Is400(t *testing.T) {
	t.Parallel()
	for _, payload := range []string{
		`{"body":"isi","audience":"all"}`,
		`{"title":"Judul","audience":"all"}`,
		`{"title":"Judul","body":"isi","audience":"mitra"}`,
		`{"title":"Judul","body":"isi","audience":"all","scheduled_at":"not-a-date"}`,
	} {
		repo := &stubBroadcastRepo{}
		app := newAdminBroadcastTestApp(t, repo, &stubBroadcastNotifier{})
		status, _ := doBroadcastRequest(t, app, "POST", "/api/v1/admin/broadcasts", payload)
		assert.Equal(t, fiber.StatusBadRequest, status, payload)
		assert.Nil(t, repo.created)
	}
}

func TestAdminHandler_CreateBroadcast_PastScheduledAt_Is422(t *testing.T) {
	t.Parallel()
	repo := &stubBroadcastRepo{}
	app := newAdminBroadcastTestApp(t, repo, &stubBroadcastNotifier{})
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)

	status, body := doBroadcastRequest(t, app, "POST", "/api/v1/admin/broadcasts",
		`{"title":"Judul","body":"isi","audience":"all","scheduled_at":"`+past+`"}`)

	assert.Equal(t, fiber.StatusUnprocessableEntity, status)
	assert.Equal(t, "INVALID_STATUS_TRANSITION", body["code"])
	assert.Nil(t, repo.created)
}

func TestAdminHandler_ListBroadcasts_ReturnsHistory(t *testing.T) {
	t.Parallel()
	repo := &stubBroadcastRepo{
		rows: []*entity.Broadcast{
			{ID: 6, Title: "Maintenance", Body: "Down.", Audience: "provider", Status: "scheduled", CreatedBy: 7},
			{ID: 5, Title: "Promo", Body: "Diskon.", Audience: "all", Status: "sent", RecipientCount: 120, CreatedBy: 7},
		},
		total: 2,
	}
	app := newAdminBroadcastTestApp(t, repo, &stubBroadcastNotifier{})

	status, body := doBroadcastRequest(t, app, "GET", "/api/v1/admin/broadcasts?page=1&limit=20", "")

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	rows := data["data"].([]interface{})
	require.Len(t, rows, 2)
	assert.Equal(t, "scheduled", rows[0].(map[string]interface{})["status"])
	assert.Equal(t, float64(2), data["pagination"].(map[string]interface{})["total"])
}

func TestAdminHandler_EstimateBroadcastRecipients_ReturnsCount(t *testing.T) {
	t.Parallel()
	repo := &stubBroadcastRepo{count: 28}
	app := newAdminBroadcastTestApp(t, repo, &stubBroadcastNotifier{})

	status, body := doBroadcastRequest(t, app, "GET", "/api/v1/admin/broadcasts/estimate?audience=provider", "")

	assert.Equal(t, fiber.StatusOK, status)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "provider", data["audience"])
	assert.Equal(t, float64(28), data["recipient_count"])
}

func TestAdminHandler_EstimateBroadcastRecipients_InvalidAudience_Is400(t *testing.T) {
	t.Parallel()
	app := newAdminBroadcastTestApp(t, &stubBroadcastRepo{}, &stubBroadcastNotifier{})

	status, _ := doBroadcastRequest(t, app, "GET", "/api/v1/admin/broadcasts/estimate?audience=everyone", "")

	assert.Equal(t, fiber.StatusBadRequest, status)
}

func TestAdminHandler_CancelBroadcast_Scheduled_Cancels(t *testing.T) {
	t.Parallel()
	repo := &stubBroadcastRepo{cancelled: &entity.Broadcast{ID: 6, Title: "M", Body: "D", Audience: "provider", Status: "cancelled", CreatedBy: 7}}
	app := newAdminBroadcastTestApp(t, repo, &stubBroadcastNotifier{})

	status, body := doBroadcastRequest(t, app, "POST", "/api/v1/admin/broadcasts/6/cancel", "")

	assert.Equal(t, fiber.StatusOK, status)
	assert.Equal(t, "cancelled", body["data"].(map[string]interface{})["status"])
}

func TestAdminHandler_CancelBroadcast_SentRow_Is422(t *testing.T) {
	t.Parallel()
	repo := &stubBroadcastRepo{cancelErr: apperror.NewDomainError(apperror.ErrInvalidTransition, "INVALID_STATUS_TRANSITION", "only scheduled broadcasts can be cancelled")}
	app := newAdminBroadcastTestApp(t, repo, &stubBroadcastNotifier{})

	status, body := doBroadcastRequest(t, app, "POST", "/api/v1/admin/broadcasts/5/cancel", "")

	assert.Equal(t, fiber.StatusUnprocessableEntity, status)
	assert.Equal(t, "INVALID_STATUS_TRANSITION", body["code"])
}

func TestAdminHandler_CancelBroadcast_MissingRow_Is404(t *testing.T) {
	t.Parallel()
	repo := &stubBroadcastRepo{cancelErr: apperror.NewDomainError(apperror.ErrNotFound, "BROADCAST_NOT_FOUND", "broadcast not found")}
	app := newAdminBroadcastTestApp(t, repo, &stubBroadcastNotifier{})

	status, _ := doBroadcastRequest(t, app, "POST", "/api/v1/admin/broadcasts/99/cancel", "")

	assert.Equal(t, fiber.StatusNotFound, status)
}

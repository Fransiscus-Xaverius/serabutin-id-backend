package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

type NotificationHandler struct {
	usecase *usecase.NotificationUsecase
}

func NewNotificationHandler(uc *usecase.NotificationUsecase) *NotificationHandler {
	return &NotificationHandler{usecase: uc}
}

func (h *NotificationHandler) List(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	items, total, err := h.usecase.List(c.Context(), userID, page, limit)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	data := make([]dto.NotificationResponse, len(items))
	for i, n := range items {
		data[i] = toNotificationResponse(n)
	}
	totalPages := total / int64(limit)
	if total%int64(limit) > 0 {
		totalPages++
	}
	return response.Success(c, fiber.StatusOK, dto.NotificationListResponse{
		Data: data,
		Pagination: dto.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func (h *NotificationHandler) MarkRead(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_ID", "invalid notification ID"))
	}
	if err := h.usecase.MarkRead(c.Context(), userID, uint(id)); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, map[string]bool{"read": true})
}

// Stream is a server-sent events endpoint. Clients reconnect automatically
// (EventSource) with Last-Event-ID; missed rows are replayed first, then the
// connection is held open polling for new rows until ~25s (under the 30s
// global timeout) so the client reconnects and resumes.
func (h *NotificationHandler) Stream(c fiber.Ctx) error {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"))
	}
	var sinceID uint64
	if v := c.Get("Last-Event-ID"); v != "" {
		sinceID, _ = strconv.ParseUint(v, 10, 64)
	}
	if v := c.Query("since_id"); v != "" && sinceID == 0 {
		sinceID, _ = strconv.ParseUint(v, 10, 64)
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	lastID := uint(sinceID)
	deadline := time.Now().Add(25 * time.Second)

	c.RequestCtx().SetBodyStreamWriter(func(w *bufio.Writer) {
		write := func(n *entity.Notification) bool {
			payload, _ := json.Marshal(toNotificationResponse(n))
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", n.ID, n.Type, payload); err != nil {
				return false
			}
			if err := w.Flush(); err != nil {
				return false
			}
			return true
		}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			items, err := h.usecase.ListSince(c.Context(), userID, lastID, 50)
			if err == nil {
				for _, n := range items {
					if !write(n) {
						return
					}
					lastID = n.ID
				}
			}
			if time.Now().After(deadline) {
				return
			}
			select {
			case <-c.Context().Done():
				return
			case <-ticker.C:
			}
		}
	})
	return nil
}

func toNotificationResponse(n *entity.Notification) dto.NotificationResponse {
	resp := dto.NotificationResponse{
		ID:        n.ID,
		UserID:    n.UserID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		Data:      n.Data,
		CreatedAt: n.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if n.ReadAt != nil {
		s := n.ReadAt.Format("2006-01-02T15:04:05Z")
		resp.ReadAt = &s
	}
	return resp
}

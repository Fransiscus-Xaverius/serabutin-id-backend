package response

import (
	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Message   string      `json:"message,omitempty"`
	RequestID string      `json:"request_id"`
}

func Success(c fiber.Ctx, status int, data interface{}) error {
	return c.Status(status).JSON(&APIResponse{
		Success:   true,
		Data:      data,
		RequestID: requestctx.GetRequestID(c),
	})
}

func SuccessMessage(c fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(&APIResponse{
		Success:   true,
		Message:   message,
		RequestID: requestctx.GetRequestID(c),
	})
}

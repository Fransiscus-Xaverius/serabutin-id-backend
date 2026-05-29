package requestctx

import (
	"github.com/gofiber/fiber/v3"
)

const (
	RequestIDKey = "request_id"
	UserIDKey    = "user_id"
	UserEmailKey = "user_email"
)

func GetRequestID(c fiber.Ctx) string {
	id, ok := c.Locals(RequestIDKey).(string)
	if !ok {
		return ""
	}
	return id
}

func SetRequestID(c fiber.Ctx, id string) {
	c.Locals(RequestIDKey, id)
}

func GetUserID(c fiber.Ctx) uint {
	id, ok := c.Locals(UserIDKey).(uint)
	if !ok {
		return 0
	}
	return id
}

func SetUserID(c fiber.Ctx, id uint) {
	c.Locals(UserIDKey, id)
}

func GetUserEmail(c fiber.Ctx) string {
	email, ok := c.Locals(UserEmailKey).(string)
	if !ok {
		return ""
	}
	return email
}

func SetUserEmail(c fiber.Ctx, email string) {
	c.Locals(UserEmailKey, email)
}

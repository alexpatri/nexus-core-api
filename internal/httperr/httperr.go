package httperr

import "github.com/gofiber/fiber/v3"

type detail struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

func Send(c fiber.Ctx, status int, code string, params map[string]any) error {
	return c.Status(status).JSON(fiber.Map{"error": detail{Code: code, Params: params}})
}

package httperr

import "github.com/gofiber/fiber/v3"

type detail struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

func Send(c fiber.Ctx, status int, code string, params map[string]any) error {
	return c.Status(status).JSON(fiber.Map{"error": detail{Code: code, Params: params}})
}

// SendInput é o 400 de corpo inválido; field aponta o campo com tipo errado,
// ou fica vazio quando o corpo nem é JSON.
func SendInput(c fiber.Ctx, code, field string) error {
	var params map[string]any
	if field != "" {
		params = map[string]any{"field": field}
	}
	return Send(c, fiber.StatusBadRequest, code, params)
}

// SendViolations é o erro de regra de ficha: o código diz o que falhou e as
// violações dizem onde.
func SendViolations(c fiber.Ctx, status int, code string, violations any) error {
	return c.Status(status).JSON(fiber.Map{"error": detail{Code: code}, "violations": violations})
}

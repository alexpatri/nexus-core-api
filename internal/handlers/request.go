package handlers

import (
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v3"
)

// Em erro, devolve o campo com tipo errado, ou "" se o corpo nem é JSON.
func decodeBody(c fiber.Ctx, v any) (string, bool) {
	err := json.Unmarshal(c.Body(), v)
	if err == nil {
		return "", true
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return typeErr.Field, false
	}
	return "", false
}

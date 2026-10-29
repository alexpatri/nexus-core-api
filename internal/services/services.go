package services

import (
	"encoding/json"
	"errors"

	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/httperr"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func ownerFromCtx(c fiber.Ctx) (primitive.ObjectID, error) {
	raw, err := auth.UserID(c)
	if err != nil {
		return primitive.NilObjectID, err
	}
	return primitive.ObjectIDFromHex(raw)
}

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

func inputError(c fiber.Ctx, code, field string) error {
	var params map[string]any
	if field != "" {
		params = map[string]any{"field": field}
	}
	return httperr.Send(c, fiber.StatusBadRequest, code, params)
}

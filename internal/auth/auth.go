package auth

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"rpg-nexus/api/core/internal/httperr"
	"rpg-nexus/api/core/internal/models"
	"rpg-nexus/api/core/internal/repository"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	userIDLocal    = "userID"
	sessionIDLocal = "sessionID"
	touchInterval  = 24 * time.Hour
)

type SessionStore interface {
	FindActive(ctx context.Context, tokenHash string, now time.Time) (models.Session, error)
	Touch(ctx context.Context, id primitive.ObjectID, now, expiresAt time.Time) error
}

func Unauthenticated(c fiber.Ctx) error {
	return httperr.Send(c, fiber.StatusUnauthorized, "auth.unauthenticated", nil)
}

func Middleware(sessions SessionStore, ttl time.Duration) fiber.Handler {
	return func(c fiber.Ctx) error {
		token, ok := strings.CutPrefix(c.Get(fiber.HeaderAuthorization), "Bearer ")
		if !ok || token == "" {
			return Unauthenticated(c)
		}

		now := time.Now()
		s, err := sessions.FindActive(c.Context(), HashToken(token), now)
		if errors.Is(err, repository.ErrNotFound) {
			return Unauthenticated(c)
		}
		// Falha do banco não pode virar 401: o app apaga o token ao receber 401.
		if err != nil {
			return c.SendStatus(fiber.StatusInternalServerError)
		}

		if now.Sub(s.LastUsedAt) > touchInterval {
			if err := sessions.Touch(c.Context(), s.ID, now, now.Add(ttl)); err != nil {
				log.Printf("auth: renovar sessão %s: %v", s.ID.Hex(), err)
			}
		}

		c.Locals(userIDLocal, s.UserID.Hex())
		c.Locals(sessionIDLocal, s.ID)
		return c.Next()
	}
}

func UserID(c fiber.Ctx) (string, error) {
	raw, ok := c.Locals(userIDLocal).(string)
	if !ok || raw == "" {
		return "", errors.New("user not in context")
	}
	return raw, nil
}

func SessionID(c fiber.Ctx) (primitive.ObjectID, error) {
	id, ok := c.Locals(sessionIDLocal).(primitive.ObjectID)
	if !ok {
		return primitive.NilObjectID, errors.New("session not in context")
	}
	return id, nil
}

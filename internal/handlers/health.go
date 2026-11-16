package handlers

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
)

func HealthHandler(ping func(context.Context) error) fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
		defer cancel()
		if err := ping(ctx); err != nil {
			log.Printf("health: %v", err)
			return c.SendStatus(fiber.StatusServiceUnavailable)
		}
		return c.SendStatus(fiber.StatusOK)
	}
}

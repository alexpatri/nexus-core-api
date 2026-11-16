package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestHealth(t *testing.T) {
	cases := []struct {
		name string
		ping error
		want int
	}{
		{"banco no ar", nil, http.StatusOK},
		{"banco fora", errors.New("server selection timeout"), http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/health", HealthHandler(func(context.Context) error { return tc.ping }))
			if r := do(t, app, http.MethodGet, "/health", "", ""); r.status != tc.want {
				t.Errorf("status = %d, quer %d", r.status, tc.want)
			}
		})
	}
}

package api

import (
	"io"
	"net"
	"net/http"
	"testing"

	"rpg-nexus/api/core/internal/config"

	"github.com/gofiber/fiber/v3"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name   string
		header string
		sent   string
		want   string
	}{
		{"sem cabeçalho configurado", "", "203.0.113.7", "127.0.0.1"},
		{"proxy confiável", "X-Real-IP", "203.0.113.7", "203.0.113.7"},
		{"proxy repassa a cadeia", "X-Forwarded-For", "203.0.113.7, 10.0.0.2", "203.0.113.7"},
		{"cabeçalho inválido", "X-Real-IP", "lixo", "127.0.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ipSeenBy(t, config.ServerConfig{ProxyHeader: tc.header}, tc.header, tc.sent)
			if got != tc.want {
				t.Errorf("c.IP() = %q, quer %q", got, tc.want)
			}
		})
	}
}

// app.Test usa uma conexão falsa, de 0.0.0.0, fora de qualquer faixa confiável:
// serve para o caso do cliente que fala direto com a API.
func TestClientIPFromUntrustedPeer(t *testing.T) {
	app := fiber.New(serverConfig(config.ServerConfig{ProxyHeader: "X-Real-IP"}))
	app.Get("/ip", func(c fiber.Ctx) error { return c.SendString(c.IP()) })

	req, _ := http.NewRequest(http.MethodGet, "/ip", nil)
	req.Header.Set("X-Real-IP", "203.0.113.7")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := io.ReadAll(resp.Body); string(body) == "203.0.113.7" {
		t.Error("cabeçalho aceito de quem não é proxy confiável")
	}
}

func ipSeenBy(t *testing.T, cfg config.ServerConfig, header, value string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New(serverConfig(cfg))
	app.Get("/ip", func(c fiber.Ctx) error { return c.SendString(c.IP()) })
	go func() { _ = app.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}) }()
	t.Cleanup(func() { _ = app.Shutdown() })

	req, _ := http.NewRequest(http.MethodGet, "http://"+ln.Addr().String()+"/ip", nil)
	if header != "" {
		req.Header.Set(header, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

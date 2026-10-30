package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/config"
	"rpg-nexus/api/core/internal/handlers"
	"rpg-nexus/api/core/internal/httperr"
	"rpg-nexus/api/core/internal/repository"
	"rpg-nexus/api/core/internal/rules"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"go.mongodb.org/mongo-driver/mongo"
)

func NewApp(cfg *config.Config, db *mongo.Database, registry *rules.Registry) (*fiber.App, error) {
	userRepo := repository.NewUser(db)
	sessionRepo := repository.NewSession(db)
	charRepo := repository.NewCharacter(db)

	ctx := context.Background()
	if err := userRepo.EnsureIndexes(ctx); err != nil {
		return nil, err
	}
	if err := sessionRepo.EnsureIndexes(ctx); err != nil {
		return nil, err
	}
	if err := charRepo.EnsureIndexes(ctx); err != nil {
		return nil, err
	}

	app := fiber.New(fiber.Config{ErrorHandler: errorHandler})
	app.Use(cors.New(cors.Config{
		AllowOrigins:  cfg.CORS.Origins,
		ExposeHeaders: []string{fiber.HeaderRetryAfter, fiber.HeaderETag},
	}))

	userSvc := handlers.NewUserHandler(userRepo, sessionRepo, cfg.Auth.SessionTTL)
	charSvc := handlers.NewCharacterHandler(charRepo, registry)

	// Sessão por rota ou por prefixo, e não num grupo raiz: assim rota
	// inexistente responde 404, e não 401.
	session := auth.Middleware(sessionRepo, cfg.Auth.SessionTTL)

	app.Post("/auth/register", rateLimit(5, ipKey), userSvc.RegisterHandler)
	app.Post("/auth/login", rateLimit(10, ipKey), rateLimit(5, emailKey), userSvc.LoginHandler)
	app.Post("/auth/logout", session, userSvc.LogoutHandler)
	app.Post("/auth/logout-all", session, userSvc.LogoutAllHandler)

	me := app.Group("/me", session)
	me.Get("", userSvc.MeHandler)
	me.Patch("", userSvc.UpdateMeHandler)
	me.Put("/password", userSvc.ChangePasswordHandler)

	characters := app.Group("/characters", session)
	characters.Get("", charSvc.ListCharactersHandler)
	characters.Post("", charSvc.CreateCharacterHandler)
	characters.Delete("/:id", charSvc.DeleteCharacterHandler)

	sheets := app.Group("/sheets", session)
	sheets.Get("/:id", charSvc.GetSheetHandler)
	sheets.Put("/:id", charSvc.PutSheetHandler)
	sheets.Post("/:id/finalize", charSvc.FinalizeSheetHandler)

	return app, nil
}

func errorHandler(c fiber.Ctx, err error) error {
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return c.SendStatus(fe.Code)
	}
	log.Printf("%s %s: %v", c.Method(), c.Path(), err)
	return c.SendStatus(fiber.StatusInternalServerError)
}

func rateLimit(n int, key func(fiber.Ctx) string) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          n,
		Expiration:   time.Minute,
		KeyGenerator: key,
		LimitReached: func(c fiber.Ctx) error {
			return httperr.Send(c, fiber.StatusTooManyRequests, "auth.rateLimited", nil)
		},
	})
}

func ipKey(c fiber.Ctx) string {
	return c.IP()
}

// Corpo sem e-mail válido cai numa chave só, que o limite por IP já cobre.
func emailKey(c fiber.Ctx) string {
	var in struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(c.Body(), &in)
	email, _ := auth.NormalizeEmail(in.Email)
	return email
}

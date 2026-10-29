package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/config"
	"rpg-nexus/api/core/internal/httperr"
	"rpg-nexus/api/core/internal/repository"
	"rpg-nexus/api/core/internal/services"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"go.mongodb.org/mongo-driver/mongo"
)

func NewApp(cfg *config.Config, db *mongo.Database) (*fiber.App, error) {
	userRepo := repository.NewUser(db)
	sessionRepo := repository.NewSession(db)
	charRepo := repository.NewCharacter(db)
	campRepo := repository.NewCampaign(db)

	ctx := context.Background()
	if err := userRepo.EnsureIndexes(ctx); err != nil {
		return nil, err
	}
	if err := sessionRepo.EnsureIndexes(ctx); err != nil {
		return nil, err
	}

	app := fiber.New(fiber.Config{ErrorHandler: errorHandler})
	app.Use(cors.New(cors.Config{
		AllowOrigins:  cfg.CORS.Origins,
		ExposeHeaders: []string{fiber.HeaderRetryAfter},
	}))

	userSvc := services.NewUserService(userRepo, sessionRepo, cfg.Auth.SessionTTL)
	charSvc := services.NewCharacterService(charRepo)
	campSvc := services.NewCampaignService(campRepo)

	// rotas públicas
	app.Post("/auth/register", rateLimit(5, ipKey), userSvc.RegisterHandler)
	app.Post("/auth/login", rateLimit(10, ipKey), rateLimit(5, emailKey), userSvc.LoginHandler)

	// rotas privadas (exigem sessão)
	api := app.Group("", auth.Middleware(sessionRepo, cfg.Auth.SessionTTL))

	api.Post("/auth/logout", userSvc.LogoutHandler)
	api.Post("/auth/logout-all", userSvc.LogoutAllHandler)
	api.Get("/me", userSvc.MeHandler)
	api.Patch("/me", userSvc.UpdateMeHandler)
	api.Put("/me/password", userSvc.ChangePasswordHandler)

	api.Get("/characters", charSvc.GetCharactersHandler)

	characterGroup := api.Group("/character")
	characterGroup.Get("/:id", charSvc.GetCharacterHandler)
	characterGroup.Post("/", charSvc.PostCharacterHandler)
	characterGroup.Put("/:id", charSvc.PutCharacterHandler)
	characterGroup.Delete("/:id", charSvc.DeleteCharacterHandler)

	api.Get("/campaigns", campSvc.GetCampaignsHandler)

	campaignGroup := api.Group("/campaign")
	campaignGroup.Post("/", campSvc.CreateCampaignHandler)
	campaignGroup.Post("/:id", campSvc.GetCampaignHandler)
	campaignGroup.Put("/:id", campSvc.PutCampaignHandler)
	campaignGroup.Delete("/:id", campSvc.DeleteCampaignHandler)

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

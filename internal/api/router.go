package api

import (
	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/config"
	"rpg-nexus/api/core/internal/repository"
	"rpg-nexus/api/core/internal/services"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"go.mongodb.org/mongo-driver/mongo"
)

func NewApp(cfg *config.Config, db *mongo.Database) (*fiber.App, error) {
	app := fiber.New()
	app.Use(cors.New())

	userRepo := repository.NewUser(db)
	charRepo := repository.NewCharacter(db)
	campRepo := repository.NewCampaign(db)

	userSvc := services.NewUserService(userRepo, cfg.Auth.Secret)
	charSvc := services.NewCharacterService(charRepo)
	campSvc := services.NewCampaignService(campRepo)

	// rotas públicas
	userGroup := app.Group("/user")
	userGroup.Post("/", userSvc.CreateUserHandler)
	userGroup.Post("/login", userSvc.ValidateUserHandler)

	// rotas privadas (exigem JWT)
	api := app.Group("", auth.Middleware(cfg.Auth.Secret))

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

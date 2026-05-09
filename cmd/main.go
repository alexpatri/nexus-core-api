package main

import (
	"log"

	"rpg-nexus/api/core/internal/api"
	"rpg-nexus/api/core/internal/config"
	"rpg-nexus/api/core/internal/database"
)

func main() {
	cfg := config.LoadConfig()

	db, err := database.Connect(cfg.DB)
	if err != nil {
		log.Fatalf("falha ao conectar ao banco: %v", err)
	}

	app, err := api.NewApp(cfg, db)
	if err != nil {
		log.Fatalf("falha ao iniciar a aplicação: %v", err)
	}

	log.Fatal(app.Listen(":" + cfg.Server.Port))
}

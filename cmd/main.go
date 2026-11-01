package main

import (
	"log"

	"github.com/alexpatri/nexus-systems-api/data"

	"rpg-nexus/api/core/internal/api"
	"rpg-nexus/api/core/internal/config"
	"rpg-nexus/api/core/internal/database"
	"rpg-nexus/api/core/internal/rules"
	"rpg-nexus/api/core/internal/rules/shadowrun"
)

func main() {
	cfg := config.LoadConfig()

	db, err := database.Connect(cfg.DB)
	if err != nil {
		log.Fatalf("falha ao conectar ao banco: %v", err)
	}

	sr, err := shadowrun.New(data.Systems())
	if err != nil {
		log.Fatalf("regras: %v", err)
	}
	registry, err := rules.NewRegistry(sr)
	if err != nil {
		log.Fatalf("regras: %v", err)
	}

	app, err := api.NewApp(cfg, db, registry)
	if err != nil {
		log.Fatalf("falha ao iniciar a aplicação: %v", err)
	}

	log.Fatal(app.Listen(":" + cfg.Server.Port))
}

package services

import (
	"context"
	"encoding/json"
	"errors"

	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/models"
	"rpg-nexus/api/core/internal/repository"
	"rpg-nexus/api/core/internal/utils"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type CampaignRepository interface {
	Find(ctx context.Context, ownerID primitive.ObjectID) ([]models.Campaign, error)
	FindByID(ctx context.Context, id string) (models.Campaign, error)
	Insert(ctx context.Context, c models.Campaign) error
	UpdateByID(ctx context.Context, id string, c models.Campaign) error
	DeleteByID(ctx context.Context, id string, ownerID primitive.ObjectID) error
}

type campaignService struct {
	repo CampaignRepository
}

func NewCampaignService(repo CampaignRepository) *campaignService {
	return &campaignService{repo: repo}
}

func (svc *campaignService) CreateCampaignHandler(c fiber.Ctx) error {
	owner, err := ownerFromCtx(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "não autenticado"})
	}

	campaign, err := models.NewCampaignFromJSON(c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Erro ao parsear o JSON da requisição"})
	}

	password, err := utils.GeneratePassword(6, 2, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Erro ao gerar a senha"})
	}

	hashedPassword, err := utils.HashPassword(password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Erro ao gerar o hash da senha"})
	}

	campaign.Password = hashedPassword
	campaign.OwnerID = owner

	if err := svc.repo.Insert(c.Context(), campaign); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Erro ao inserir a campanha no banco de dados"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id":   campaign.Id,
		"pass": password,
	})
}

func (svc *campaignService) GetCampaignsHandler(c fiber.Ctx) error {
	owner, err := ownerFromCtx(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "não autenticado"})
	}

	campaigns, err := svc.repo.Find(c.Context(), owner)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Algo deu errado"})
	}
	return c.JSON(fiber.Map{"campaigns": campaigns})
}

func (svc *campaignService) GetCampaignHandler(c fiber.Ctx) error {
	id := c.Params("id")
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ID do parâmetro é obrigatório"})
	}

	var body struct {
		Password string `json:"pass"`
	}
	if err := json.Unmarshal(c.Body(), &body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Body inválido"})
	}

	campaign, err := svc.repo.FindByID(c.Context(), id)
	if errors.Is(err, repository.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Campanha não encontrada"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Algo deu errado"})
	}

	if !utils.VerifyPassword(body.Password, campaign.Password) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Senha incorreta"})
	}

	if campaign.Characters == nil {
		campaign.Characters = []models.Character{}
	}
	if campaign.Messages == nil {
		campaign.Messages = []models.Message{}
	}

	return c.JSON(campaign)
}

func (svc *campaignService) PutCampaignHandler(c fiber.Ctx) error {
	if _, err := auth.UserID(c); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "não autenticado"})
	}

	var campaign models.Campaign
	if err := json.Unmarshal(c.Body(), &campaign); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Formato JSON inválido para a campanha.",
		})
	}

	err := svc.repo.UpdateByID(c.Context(), c.Params("id"), campaign)
	if errors.Is(err, repository.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Campanha não encontrada.",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Erro interno ao atualizar a campanha.",
		})
	}
	return c.SendStatus(fiber.StatusOK)
}

func (svc *campaignService) DeleteCampaignHandler(c fiber.Ctx) error {
	owner, err := ownerFromCtx(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "não autenticado"})
	}

	err = svc.repo.DeleteByID(c.Context(), c.Params("id"), owner)
	if errors.Is(err, repository.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Campanha não encontrada.",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Erro interno ao excluir a campanha.",
		})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

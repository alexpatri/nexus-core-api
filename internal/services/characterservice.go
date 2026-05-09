package services

import (
	"context"
	"encoding/json"
	"errors"

	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/models"
	"rpg-nexus/api/core/internal/repository"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type CharacterRepository interface {
	Find(ctx context.Context, ownerID primitive.ObjectID) (models.Characters, error)
	FindByID(ctx context.Context, id string, ownerID primitive.ObjectID) (models.Character, error)
	Insert(ctx context.Context, c models.Character) error
	UpdateByID(ctx context.Context, id string, ownerID primitive.ObjectID, c models.Character) error
	DeleteByID(ctx context.Context, id string, ownerID primitive.ObjectID) error
}

type characterService struct {
	repo CharacterRepository
}

func NewCharacterService(repo CharacterRepository) *characterService {
	return &characterService{repo: repo}
}

func ownerFromCtx(c fiber.Ctx) (primitive.ObjectID, error) {
	raw, err := auth.UserID(c)
	if err != nil {
		return primitive.NilObjectID, err
	}
	return primitive.ObjectIDFromHex(raw)
}

func (svc *characterService) GetCharactersHandler(c fiber.Ctx) error {
	owner, err := ownerFromCtx(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "não autenticado"})
	}

	characters, err := svc.repo.Find(c.Context(), owner)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Erro interno ao buscar personagens.",
		})
	}
	return c.JSON(characters)
}

func (svc *characterService) GetCharacterHandler(c fiber.Ctx) error {
	owner, err := ownerFromCtx(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "não autenticado"})
	}

	character, err := svc.repo.FindByID(c.Context(), c.Params("id"), owner)
	if errors.Is(err, repository.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Personagem não encontrado.",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Erro interno ao buscar o personagem.",
		})
	}
	return c.JSON(character)
}

func (svc *characterService) PostCharacterHandler(c fiber.Ctx) error {
	owner, err := ownerFromCtx(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "não autenticado"})
	}

	character, err := models.NewCharacterFromJSON(c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Formato JSON inválido para o personagem.",
		})
	}
	character.OwnerID = owner

	if err := svc.repo.Insert(c.Context(), character); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Erro interno ao salvar o personagem.",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"id": character.Id,
	})
}

func (svc *characterService) PutCharacterHandler(c fiber.Ctx) error {
	owner, err := ownerFromCtx(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "não autenticado"})
	}

	var character models.Character
	if err := json.Unmarshal(c.Body(), &character); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Formato JSON inválido para o personagem.",
		})
	}
	character.OwnerID = owner

	err = svc.repo.UpdateByID(c.Context(), c.Params("id"), owner, character)
	if errors.Is(err, repository.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Personagem não encontrado.",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Erro interno ao atualizar o personagem.",
		})
	}
	return c.SendStatus(fiber.StatusOK)
}

func (svc *characterService) DeleteCharacterHandler(c fiber.Ctx) error {
	owner, err := ownerFromCtx(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "não autenticado"})
	}

	err = svc.repo.DeleteByID(c.Context(), c.Params("id"), owner)
	if errors.Is(err, repository.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Personagem não encontrado.",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Erro interno ao excluir o personagem.",
		})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

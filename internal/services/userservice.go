package services

import (
	"context"
	"errors"

	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/models"
	"rpg-nexus/api/core/internal/repository"
	"rpg-nexus/api/core/internal/utils"

	"github.com/gofiber/fiber/v3"
)

type UserRepository interface {
	Insert(ctx context.Context, u models.User) error
	FindByEmail(ctx context.Context, email string) (models.User, error)
}

type userService struct {
	repo   UserRepository
	secret string
}

func NewUserService(repo UserRepository, secret string) *userService {
	return &userService{repo: repo, secret: secret}
}

type userResponse struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func toResponse(u models.User) userResponse {
	return userResponse{
		Id:    u.Id.Hex(),
		Name:  u.Name,
		Email: u.Email,
	}
}

func (svc *userService) sessionResponse(u models.User) (fiber.Map, error) {
	token, err := auth.GenerateToken(u.Id.Hex(), svc.secret)
	if err != nil {
		return nil, err
	}
	return fiber.Map{
		"token": token,
		"user":  toResponse(u),
	}, nil
}

func (svc *userService) CreateUserHandler(c fiber.Ctx) error {
	user, err := models.NewUserFromJSON(c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Body inválido"})
	}

	exists, err := svc.isEmailExists(c.Context(), user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Algo deu errado"})
	}
	if exists {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Email já cadastrado. Utilize outro email ou faça login"})
	}

	if err := svc.repo.Insert(c.Context(), user); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Algo deu errado"})
	}

	resp, err := svc.sessionResponse(user)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Erro ao gerar sessão"})
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

func (svc *userService) ValidateUserHandler(c fiber.Ctx) error {
	input, err := models.ConvertJSONToUser(c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Body inválido"})
	}

	stored, err := svc.repo.FindByEmail(c.Context(), input.Email)
	if errors.Is(err, repository.ErrNotFound) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email ou senha incorretos"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Algo deu errado"})
	}

	if !utils.VerifyPassword(input.Password, stored.Password) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email ou senha incorretos"})
	}

	resp, err := svc.sessionResponse(stored)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Erro ao gerar sessão"})
	}
	return c.Status(fiber.StatusOK).JSON(resp)
}

func (svc *userService) isEmailExists(ctx context.Context, email string) (bool, error) {
	_, err := svc.repo.FindByEmail(ctx, email)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

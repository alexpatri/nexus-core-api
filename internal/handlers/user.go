package handlers

import (
	"context"
	"errors"
	"strings"
	"time"

	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/httperr"
	"rpg-nexus/api/core/internal/models"
	"rpg-nexus/api/core/internal/repository"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type UserRepository interface {
	Insert(ctx context.Context, u models.User) error
	FindByEmail(ctx context.Context, email string) (models.User, error)
	FindByID(ctx context.Context, id primitive.ObjectID) (models.User, error)
	UpdateName(ctx context.Context, id primitive.ObjectID, name string) (models.User, error)
	UpdatePasswordHash(ctx context.Context, id primitive.ObjectID, hash string) error
}

type SessionRepository interface {
	Insert(ctx context.Context, s models.Session) error
	Delete(ctx context.Context, id primitive.ObjectID) error
	DeleteByUser(ctx context.Context, userID primitive.ObjectID) error
	DeleteByUserExcept(ctx context.Context, userID, keep primitive.ObjectID) error
}

type userHandler struct {
	users      UserRepository
	sessions   SessionRepository
	sessionTTL time.Duration
}

func NewUserHandler(users UserRepository, sessions SessionRepository, sessionTTL time.Duration) *userHandler {
	return &userHandler{users: users, sessions: sessions, sessionTTL: sessionTTL}
}

type userResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func toResponse(u models.User) userResponse {
	return userResponse{ID: u.ID.Hex(), Name: u.Name, Email: u.Email}
}

func invalidInput(c fiber.Ctx, field string) error {
	return httperr.SendInput(c, "auth.invalidInput", field)
}

func invalidCredentials(c fiber.Ctx) error {
	return httperr.Send(c, fiber.StatusUnauthorized, "auth.invalidCredentials", nil)
}

func weakPassword(c fiber.Ctx) error {
	return httperr.Send(c, fiber.StatusBadRequest, "auth.weakPassword", nil)
}

func (svc *userHandler) startSession(c fiber.Ctx, status int, u models.User, deviceName string) error {
	token := auth.NewToken()
	now := time.Now()
	s := models.Session{
		ID:         primitive.NewObjectID(),
		UserID:     u.ID,
		TokenHash:  auth.HashToken(token),
		DeviceName: strings.TrimSpace(deviceName),
		CreatedAt:  now,
		LastUsedAt: now,
		ExpiresAt:  now.Add(svc.sessionTTL),
	}
	if err := svc.sessions.Insert(c.Context(), s); err != nil {
		return err
	}
	return c.Status(status).JSON(fiber.Map{"token": token, "user": toResponse(u)})
}

func (svc *userHandler) RegisterHandler(c fiber.Ctx) error {
	var in struct {
		Name       string `json:"name"`
		Email      string `json:"email"`
		Password   string `json:"password"`
		DeviceName string `json:"deviceName"`
	}
	if field, ok := decodeBody(c, &in); !ok {
		return invalidInput(c, field)
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		return invalidInput(c, "name")
	}
	email, ok := auth.NormalizeEmail(in.Email)
	if !ok {
		return invalidInput(c, "email")
	}
	if !auth.ValidPassword(in.Password) {
		return weakPassword(c)
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	u := models.User{
		ID:           primitive.NewObjectID(),
		Email:        email,
		Name:         name,
		PasswordHash: hash,
		CreatedAt:    time.Now(),
	}
	err = svc.users.Insert(c.Context(), u)
	if errors.Is(err, repository.ErrDuplicate) {
		return httperr.Send(c, fiber.StatusConflict, "auth.emailTaken", nil)
	}
	if err != nil {
		return err
	}

	return svc.startSession(c, fiber.StatusCreated, u, in.DeviceName)
}

func (svc *userHandler) LoginHandler(c fiber.Ctx) error {
	var in struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		DeviceName string `json:"deviceName"`
	}
	if field, ok := decodeBody(c, &in); !ok {
		return invalidInput(c, field)
	}

	email, ok := auth.NormalizeEmail(in.Email)
	if !ok {
		return invalidCredentials(c)
	}
	u, err := svc.users.FindByEmail(c.Context(), email)
	if errors.Is(err, repository.ErrNotFound) {
		return invalidCredentials(c)
	}
	if err != nil {
		return err
	}
	if !auth.VerifyPassword(in.Password, u.PasswordHash) {
		return invalidCredentials(c)
	}

	return svc.startSession(c, fiber.StatusOK, u, in.DeviceName)
}

func (svc *userHandler) LogoutHandler(c fiber.Ctx) error {
	id, err := auth.SessionID(c)
	if err != nil {
		return auth.Unauthenticated(c)
	}
	if err := svc.sessions.Delete(c.Context(), id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (svc *userHandler) LogoutAllHandler(c fiber.Ctx) error {
	userID, err := auth.UserID(c)
	if err != nil {
		return auth.Unauthenticated(c)
	}
	if err := svc.sessions.DeleteByUser(c.Context(), userID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (svc *userHandler) currentUser(c fiber.Ctx) (models.User, error) {
	userID, err := auth.UserID(c)
	if err != nil {
		return models.User{}, repository.ErrNotFound
	}
	return svc.users.FindByID(c.Context(), userID)
}

func (svc *userHandler) MeHandler(c fiber.Ctx) error {
	u, err := svc.currentUser(c)
	if errors.Is(err, repository.ErrNotFound) {
		return auth.Unauthenticated(c)
	}
	if err != nil {
		return err
	}
	return c.JSON(toResponse(u))
}

func (svc *userHandler) UpdateMeHandler(c fiber.Ctx) error {
	var in struct {
		Name string `json:"name"`
	}
	if field, ok := decodeBody(c, &in); !ok {
		return invalidInput(c, field)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return invalidInput(c, "name")
	}

	userID, err := auth.UserID(c)
	if err != nil {
		return auth.Unauthenticated(c)
	}
	u, err := svc.users.UpdateName(c.Context(), userID, name)
	if errors.Is(err, repository.ErrNotFound) {
		return auth.Unauthenticated(c)
	}
	if err != nil {
		return err
	}
	return c.JSON(toResponse(u))
}

func (svc *userHandler) ChangePasswordHandler(c fiber.Ctx) error {
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if field, ok := decodeBody(c, &in); !ok {
		return invalidInput(c, field)
	}

	u, err := svc.currentUser(c)
	if errors.Is(err, repository.ErrNotFound) {
		return auth.Unauthenticated(c)
	}
	if err != nil {
		return err
	}
	// 403, e não 401: o app apaga o token ao receber 401.
	if !auth.VerifyPassword(in.CurrentPassword, u.PasswordHash) {
		return httperr.Send(c, fiber.StatusForbidden, "auth.wrongPassword", nil)
	}
	if !auth.ValidPassword(in.NewPassword) {
		return weakPassword(c)
	}

	hash, err := auth.HashPassword(in.NewPassword)
	if err != nil {
		return err
	}
	if err := svc.users.UpdatePasswordHash(c.Context(), u.ID, hash); err != nil {
		return err
	}
	sessionID, err := auth.SessionID(c)
	if err != nil {
		return auth.Unauthenticated(c)
	}
	if err := svc.sessions.DeleteByUserExcept(c.Context(), u.ID, sessionID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/httperr"
	"rpg-nexus/api/core/internal/models"
	"rpg-nexus/api/core/internal/repository"
	"rpg-nexus/api/core/internal/rules"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type CharacterRepository interface {
	Create(ctx context.Context, c models.Character, s models.Sheet) error
	List(ctx context.Context, owner primitive.ObjectID, system string) ([]models.CharacterListItem, error)
	FindSheet(ctx context.Context, owner, id primitive.ObjectID) (models.Sheet, error)
	UpdateSheet(ctx context.Context, s models.Sheet, expected int, name string) error
	Delete(ctx context.Context, owner, id primitive.ObjectID) error
}

type characterHandler struct {
	repo  CharacterRepository
	rules *rules.Registry
}

func NewCharacterHandler(repo CharacterRepository, registry *rules.Registry) *characterHandler {
	return &characterHandler{repo: repo, rules: registry}
}

type sheetResponse struct {
	models.Sheet
	Warnings []rules.Violation `json:"warnings,omitempty"`
}

func sendSheet(c fiber.Ctx, status int, s models.Sheet, warnings []rules.Violation) error {
	c.Set(fiber.HeaderETag, etag(s.Revision))
	return c.Status(status).JSON(sheetResponse{Sheet: s, Warnings: warnings})
}

func etag(revision int) string {
	return `"` + strconv.Itoa(revision) + `"`
}

func parseIfMatch(h string) (int, bool) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	n, err := strconv.Atoi(strings.Trim(h, `"`))
	return n, err == nil && n > 0
}

func isObject(j json.RawMessage) bool {
	j = bytes.TrimSpace(j)
	return len(j) > 0 && j[0] == '{'
}

func sheetNotFound(c fiber.Ctx) error {
	return httperr.Send(c, fiber.StatusNotFound, "sheet.notFound", nil)
}

func lenientFailed(c fiber.Ctx, v []rules.Violation) error {
	return httperr.SendViolations(c, fiber.StatusUnprocessableEntity, "sheet.invalid", v)
}

// summarize devolve o resumo em JSON e o nome do personagem; progress só existe
// no rascunho, e quem sabe o status é o core, não a estratégia.
func summarize(sys rules.System, data json.RawMessage, status string) (json.RawMessage, string, error) {
	summary, err := sys.Summarize(data)
	if err != nil {
		return nil, "", err
	}
	if status != models.StatusDraft {
		summary.Progress = nil
	}
	raw, err := json.Marshal(summary)
	return raw, summary.Title, err
}

func (svc *characterHandler) CreateCharacterHandler(c fiber.Ctx) error {
	owner, err := auth.UserID(c)
	if err != nil {
		return auth.Unauthenticated(c)
	}
	var in struct {
		System        string          `json:"system"`
		SystemVersion string          `json:"systemVersion"`
		Data          json.RawMessage `json:"data"`
	}
	if field, ok := decodeBody(c, &in); !ok {
		return httperr.SendInput(c, "character.invalidInput", field)
	}
	sys, err := svc.rules.Get(in.System, in.SystemVersion)
	if err != nil {
		return httperr.Send(c, fiber.StatusBadRequest, "character.unknownSystem", nil)
	}
	data := in.Data
	if len(data) == 0 || string(data) == "null" {
		data = json.RawMessage(`{}`)
	}
	if !isObject(data) {
		return httperr.SendInput(c, "character.invalidInput", "data")
	}
	if v := sys.Validate(data, rules.Lenient); len(v) > 0 {
		return lenientFailed(c, v)
	}
	summary, name, err := summarize(sys, data, models.StatusDraft)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	character := models.Character{
		ID:            primitive.NewObjectID(),
		OwnerID:       owner,
		System:        in.System,
		SystemVersion: in.SystemVersion,
		Name:          name,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	sheet := models.Sheet{
		ID:            primitive.NewObjectID(),
		CharacterID:   character.ID,
		OwnerID:       owner,
		System:        in.System,
		SystemVersion: in.SystemVersion,
		SchemaVersion: rules.SchemaVersion(sys),
		Status:        models.StatusDraft,
		Revision:      1,
		Summary:       summary,
		Data:          data,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := svc.repo.Create(c.Context(), character, sheet); err != nil {
		return err
	}

	c.Set(fiber.HeaderETag, etag(sheet.Revision))
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"character": character, "sheet": sheet})
}

func (svc *characterHandler) ListCharactersHandler(c fiber.Ctx) error {
	owner, err := auth.UserID(c)
	if err != nil {
		return auth.Unauthenticated(c)
	}
	items, err := svc.repo.List(c.Context(), owner, c.Query("system"))
	if err != nil {
		return err
	}
	if items == nil {
		items = []models.CharacterListItem{}
	}
	return c.JSON(items)
}

func (svc *characterHandler) DeleteCharacterHandler(c fiber.Ctx) error {
	owner, err := auth.UserID(c)
	if err != nil {
		return auth.Unauthenticated(c)
	}
	id, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err == nil {
		err = svc.repo.Delete(c.Context(), owner, id)
	}
	if err == nil {
		return c.SendStatus(fiber.StatusNoContent)
	}
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, primitive.ErrInvalidHex) {
		return httperr.Send(c, fiber.StatusNotFound, "character.notFound", nil)
	}
	return err
}

// load busca a ficha do usuário e a estratégia dela, já com data no formato
// atual; a migração só é gravada na próxima escrita.
func (svc *characterHandler) load(c fiber.Ctx) (models.Sheet, rules.System, error) {
	owner, err := auth.UserID(c)
	if err != nil {
		return models.Sheet{}, nil, err
	}
	id, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return models.Sheet{}, nil, repository.ErrNotFound
	}
	s, err := svc.repo.FindSheet(c.Context(), owner, id)
	if err != nil {
		return models.Sheet{}, nil, err
	}
	sys, err := svc.rules.Get(s.System, s.SystemVersion)
	if err != nil {
		return models.Sheet{}, nil, err
	}
	if current := rules.SchemaVersion(sys); s.SchemaVersion < current {
		s.Data, err = sys.(rules.Migrator).Migrate(s.Data, s.SchemaVersion)
		if err != nil {
			return models.Sheet{}, nil, err
		}
		s.SchemaVersion = current
	}
	return s, sys, nil
}

func (svc *characterHandler) loadOrFail(c fiber.Ctx) (models.Sheet, rules.System, bool, error) {
	s, sys, err := svc.load(c)
	if errors.Is(err, repository.ErrNotFound) {
		return s, sys, false, sheetNotFound(c)
	}
	if err != nil {
		return s, sys, false, err
	}
	return s, sys, true, nil
}

func (svc *characterHandler) GetSheetHandler(c fiber.Ctx) error {
	s, sys, ok, err := svc.loadOrFail(c)
	if !ok {
		return err
	}
	var warnings []rules.Violation
	if s.Status == models.StatusReady {
		warnings = sys.Validate(s.Data, rules.Strict)
	}
	return sendSheet(c, fiber.StatusOK, s, warnings)
}

// PutSheetHandler grava o estado inteiro da ficha, rascunho ou pronta. Na
// pronta, as regras do livro voltam como aviso e não bloqueiam.
func (svc *characterHandler) PutSheetHandler(c fiber.Ctx) error {
	expected, ok := parseIfMatch(c.Get(fiber.HeaderIfMatch))
	if !ok {
		return httperr.Send(c, fiber.StatusPreconditionRequired, "sheet.revisionRequired", nil)
	}
	var in struct {
		Data json.RawMessage `json:"data"`
	}
	if field, ok := decodeBody(c, &in); !ok {
		return httperr.SendInput(c, "sheet.invalidInput", field)
	}
	if !isObject(in.Data) {
		return httperr.SendInput(c, "sheet.invalidInput", "data")
	}

	s, sys, ok, err := svc.loadOrFail(c)
	if !ok {
		return err
	}
	if v := sys.Validate(in.Data, rules.Lenient); len(v) > 0 {
		return lenientFailed(c, v)
	}
	summary, name, err := summarize(sys, in.Data, s.Status)
	if err != nil {
		return err
	}
	var warnings []rules.Violation
	s.Derived = nil
	if s.Status == models.StatusReady {
		if s.Derived, err = sys.Derive(in.Data); err != nil {
			return err
		}
		warnings = sys.Validate(in.Data, rules.Strict)
	}
	s.Data = in.Data
	s.Summary = summary
	s.UpdatedAt = time.Now().UTC()

	if ok, err := svc.update(c, s, expected, name); !ok {
		return err
	}
	s.Revision = expected + 1
	return sendSheet(c, fiber.StatusOK, s, warnings)
}

func (svc *characterHandler) FinalizeSheetHandler(c fiber.Ctx) error {
	s, sys, ok, err := svc.loadOrFail(c)
	if !ok {
		return err
	}
	if h := c.Get(fiber.HeaderIfMatch); h != "" {
		if expected, ok := parseIfMatch(h); !ok || expected != s.Revision {
			return revisionConflict(c)
		}
	}
	// Repetir o finalize depois de uma falha de rede não é erro.
	if s.Status == models.StatusReady {
		return sendSheet(c, fiber.StatusOK, s, sys.Validate(s.Data, rules.Strict))
	}
	if v := sys.Validate(s.Data, rules.Strict); len(v) > 0 {
		return httperr.SendViolations(c, fiber.StatusUnprocessableEntity, "sheet.rulesViolated", v)
	}

	derived, err := sys.Derive(s.Data)
	if err != nil {
		return err
	}
	summary, name, err := summarize(sys, s.Data, models.StatusReady)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	expected := s.Revision
	s.Status = models.StatusReady
	s.Derived = derived
	s.Summary = summary
	s.UpdatedAt = now
	s.FinalizedAt = &now

	if ok, err := svc.update(c, s, expected, name); !ok {
		return err
	}
	s.Revision = expected + 1
	return sendSheet(c, fiber.StatusOK, s, nil)
}

func revisionConflict(c fiber.Ctx) error {
	return httperr.Send(c, fiber.StatusConflict, "sheet.revisionConflict", nil)
}

// update já responde 404 ou 409; ok diz se o chamador ainda deve responder.
func (svc *characterHandler) update(c fiber.Ctx, s models.Sheet, expected int, name string) (bool, error) {
	err := svc.repo.UpdateSheet(c.Context(), s, expected, name)
	switch {
	case errors.Is(err, repository.ErrConflict):
		return false, revisionConflict(c)
	case errors.Is(err, repository.ErrNotFound):
		return false, sheetNotFound(c)
	}
	return err == nil, err
}

package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/models"
	"rpg-nexus/api/core/internal/repository"
	"rpg-nexus/api/core/internal/rules"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// toy exige nome no Strict e só aceita points numérico no Lenient.
type toy struct{ version string }

type toyData struct {
	Name   string `json:"name"`
	Points int    `json:"points"`
}

func (t toy) Key() rules.Key { return rules.Key{System: "toy", Version: t.version} }

func (toy) Validate(data json.RawMessage, mode rules.Mode) []rules.Violation {
	var d toyData
	if err := json.Unmarshal(data, &d); err != nil {
		return []rules.Violation{{Step: "points", Path: "points", Code: "shape.invalid"}}
	}
	if mode == rules.Strict && d.Name == "" {
		return []rules.Violation{{Step: "concept", Path: "name", Code: "required"}}
	}
	return nil
}

func (toy) Derive(data json.RawMessage) (json.RawMessage, error) {
	var d toyData
	_ = json.Unmarshal(data, &d)
	return json.Marshal(map[string]int{"double": d.Points * 2})
}

func (toy) Summarize(data json.RawMessage) (rules.Summary, error) {
	var d toyData
	_ = json.Unmarshal(data, &d)
	return rules.Summary{Title: d.Name, Subtitle: "Brinquedo", Progress: &rules.Progress{Step: 1, Total: 2}}, nil
}

// toyV2 renomeou points para pts.
type toyV2 struct{ toy }

func (toyV2) SchemaVersion() int { return 2 }

func (toyV2) Migrate(data json.RawMessage, _ int) (json.RawMessage, error) {
	return json.RawMessage(strings.Replace(string(data), `"points"`, `"pts"`, 1)), nil
}

type fakeCharacters struct {
	mu         sync.Mutex
	characters map[primitive.ObjectID]models.Character
	sheets     map[primitive.ObjectID]models.Sheet
}

func (f *fakeCharacters) Create(_ context.Context, c models.Character, s models.Sheet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.characters[c.ID] = c
	f.sheets[s.ID] = s
	return nil
}

func (f *fakeCharacters) List(_ context.Context, owner primitive.ObjectID, system string) ([]models.CharacterListItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var items []models.CharacterListItem
	for _, c := range f.characters {
		if c.OwnerID != owner || (system != "" && c.System != system) {
			continue
		}
		item := models.CharacterListItem{Character: c}
		for _, s := range f.sheets {
			if s.CharacterID == c.ID && s.CampaignID == nil {
				item.Sheet = &models.SheetSummary{ID: s.ID, Status: s.Status, Revision: s.Revision, Summary: s.Summary, UpdatedAt: s.UpdatedAt}
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func (f *fakeCharacters) FindSheet(_ context.Context, owner, id primitive.ObjectID) (models.Sheet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sheets[id]
	if !ok || s.OwnerID != owner {
		return models.Sheet{}, repository.ErrNotFound
	}
	return s, nil
}

func (f *fakeCharacters) UpdateSheet(_ context.Context, s models.Sheet, expected int, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored, ok := f.sheets[s.ID]
	if !ok || stored.OwnerID != s.OwnerID {
		return repository.ErrNotFound
	}
	if stored.Revision != expected {
		return repository.ErrConflict
	}
	s.Revision = expected + 1
	f.sheets[s.ID] = s
	c := f.characters[s.CharacterID]
	c.Name, c.UpdatedAt = name, s.UpdatedAt
	f.characters[c.ID] = c
	return nil
}

func (f *fakeCharacters) Delete(_ context.Context, owner, id primitive.ObjectID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.characters[id]
	if !ok || c.OwnerID != owner {
		return repository.ErrNotFound
	}
	delete(f.characters, id)
	for sid, s := range f.sheets {
		if s.CharacterID == id {
			delete(f.sheets, sid)
		}
	}
	return nil
}

type charEnv struct {
	app   *fiber.App
	repo  *fakeCharacters
	alice string
	bob   string
}

func newCharEnv(t *testing.T, systems ...rules.System) charEnv {
	t.Helper()
	if len(systems) == 0 {
		systems = []rules.System{toy{version: "1"}}
	}
	registry, err := rules.NewRegistry(systems...)
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeCharacters{
		characters: map[primitive.ObjectID]models.Character{},
		sheets:     map[primitive.ObjectID]models.Sheet{},
	}
	sessions := &fakeSessions{byID: map[primitive.ObjectID]models.Session{}}
	login := func() string {
		token := auth.NewToken()
		now := time.Now()
		_ = sessions.Insert(context.Background(), models.Session{
			ID: primitive.NewObjectID(), UserID: primitive.NewObjectID(), TokenHash: auth.HashToken(token),
			CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(time.Hour),
		})
		return token
	}

	svc := NewCharacterService(repo, registry)
	app := fiber.New()
	chars := app.Group("/characters", auth.Middleware(sessions, time.Hour))
	chars.Get("", svc.ListCharactersHandler)
	chars.Post("", svc.CreateCharacterHandler)
	chars.Delete("/:id", svc.DeleteCharacterHandler)
	sheets := app.Group("/sheets", auth.Middleware(sessions, time.Hour))
	sheets.Get("/:id", svc.GetSheetHandler)
	sheets.Put("/:id", svc.PutSheetHandler)
	sheets.Post("/:id/finalize", svc.FinalizeSheetHandler)
	return charEnv{app: app, repo: repo, alice: login(), bob: login()}
}

type charResult struct {
	status int
	etag   string
	body   map[string]any
	list   []map[string]any
}

func (e charEnv) do(t *testing.T, method, path, token, ifMatch, body string) charResult {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	var r charResult
	r.status, r.etag = resp.StatusCode, resp.Header.Get("ETag")
	var raw json.RawMessage
	_ = json.NewDecoder(resp.Body).Decode(&raw)
	if len(raw) > 0 && raw[0] == '[' {
		_ = json.Unmarshal(raw, &r.list)
	} else {
		_ = json.Unmarshal(raw, &r.body)
	}
	return r
}

func (r charResult) code() string { return errCode(result{body: r.body}) }

func (e charEnv) create(t *testing.T, token, data string) (characterID, sheetID string) {
	t.Helper()
	r := e.do(t, http.MethodPost, "/characters", token, "", `{"system":"toy","systemVersion":"1","data":`+data+`}`)
	if r.status != fiber.StatusCreated {
		t.Fatalf("create: %d %v", r.status, r.body)
	}
	return r.body["character"].(map[string]any)["id"].(string), r.body["sheet"].(map[string]any)["id"].(string)
}

func TestCreateCharacter(t *testing.T) {
	e := newCharEnv(t)

	r := e.do(t, http.MethodPost, "/characters", e.alice, "", `{"system":"toy","systemVersion":"1","data":{"name":"Grok"}}`)
	if r.status != fiber.StatusCreated || r.etag != `"1"` {
		t.Fatalf("got %d etag %q: %v", r.status, r.etag, r.body)
	}
	character := r.body["character"].(map[string]any)
	sheet := r.body["sheet"].(map[string]any)
	if character["name"] != "Grok" || sheet["status"] != "draft" || sheet["revision"] != 1.0 || sheet["campaignId"] != nil {
		t.Errorf("character %v, sheet %v", character, sheet)
	}
	if sheet["summary"].(map[string]any)["progress"] == nil {
		t.Error("rascunho sem progress")
	}
	if _, ok := sheet["ownerId"]; ok {
		t.Error("resposta expõe ownerId")
	}

	r = e.do(t, http.MethodPost, "/characters", e.alice, "", `{"system":"toy","systemVersion":"1"}`)
	if r.status != fiber.StatusCreated || string(mustJSON(r.body["sheet"].(map[string]any)["data"])) != "{}" {
		t.Errorf("sem data: %d %v", r.status, r.body)
	}

	cases := []struct {
		name, body, code string
		status           int
	}{
		{"sistema desconhecido", `{"system":"toy","systemVersion":"9"}`, "character.unknownSystem", 400},
		{"data não é objeto", `{"system":"toy","systemVersion":"1","data":[1]}`, "character.invalidInput", 400},
		{"lenient recusa", `{"system":"toy","systemVersion":"1","data":{"points":"x"}}`, "sheet.invalid", 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := e.do(t, http.MethodPost, "/characters", e.alice, "", tc.body)
			if r.status != tc.status || r.code() != tc.code {
				t.Errorf("got %d %q, want %d %q", r.status, r.code(), tc.status, tc.code)
			}
		})
	}
	if v, _ := e.do(t, http.MethodPost, "/characters", e.alice, "", cases[2].body).body["violations"].([]any); len(v) != 1 {
		t.Errorf("violations = %v", v)
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestListCharacters(t *testing.T) {
	e := newCharEnv(t)
	e.create(t, e.alice, `{"name":"Grok"}`)

	r := e.do(t, http.MethodGet, "/characters", e.alice, "", "")
	if r.status != fiber.StatusOK || len(r.list) != 1 {
		t.Fatalf("got %d %v", r.status, r.list)
	}
	sheet := r.list[0]["sheet"].(map[string]any)
	if _, ok := sheet["data"]; ok || sheet["summary"] == nil {
		t.Errorf("sheet da lista = %v", sheet)
	}

	if r := e.do(t, http.MethodGet, "/characters?system=dnd", e.alice, "", ""); len(r.list) != 0 {
		t.Errorf("filtro por sistema: %v", r.list)
	}
	if r := e.do(t, http.MethodGet, "/characters", e.bob, "", ""); r.status != fiber.StatusOK || r.list == nil || len(r.list) != 0 {
		t.Errorf("outro usuário: %d %v", r.status, r.list)
	}
}

func TestGetSheet(t *testing.T) {
	e := newCharEnv(t)
	_, sheetID := e.create(t, e.alice, `{"name":"Grok"}`)

	r := e.do(t, http.MethodGet, "/sheets/"+sheetID, e.alice, "", "")
	if r.status != fiber.StatusOK || r.etag != `"1"` || r.body["data"].(map[string]any)["name"] != "Grok" {
		t.Errorf("got %d %q %v", r.status, r.etag, r.body)
	}
	if _, ok := r.body["warnings"]; ok {
		t.Error("rascunho com warnings")
	}
	for _, path := range []string{"/sheets/" + primitive.NewObjectID().Hex(), "/sheets/nada"} {
		if r := e.do(t, http.MethodGet, path, e.alice, "", ""); r.status != fiber.StatusNotFound || r.code() != "sheet.notFound" {
			t.Errorf("%s: got %d %q", path, r.status, r.code())
		}
	}
	if r := e.do(t, http.MethodGet, "/sheets/"+sheetID, e.bob, "", ""); r.status != fiber.StatusNotFound {
		t.Errorf("ficha de outro usuário: %d", r.status)
	}
}

func TestPutDraft(t *testing.T) {
	e := newCharEnv(t)
	_, sheetID := e.create(t, e.alice, `{}`)
	path := "/sheets/" + sheetID

	if r := e.do(t, http.MethodPut, path, e.alice, "", `{"data":{}}`); r.status != fiber.StatusPreconditionRequired || r.code() != "sheet.revisionRequired" {
		t.Errorf("sem If-Match: %d %q", r.status, r.code())
	}
	if r := e.do(t, http.MethodPut, path, e.alice, `"1"`, `{"data":{"points":"x"}}`); r.status != fiber.StatusUnprocessableEntity {
		t.Errorf("lenient: %d", r.status)
	}
	if r := e.do(t, http.MethodPut, path, e.alice, `"1"`, `{"data":"x"}`); r.code() != "sheet.invalidInput" {
		t.Errorf("data não é objeto: %d %q", r.status, r.code())
	}

	r := e.do(t, http.MethodPut, path, e.alice, `"1"`, `{"data":{"name":"Grok","points":3}}`)
	if r.status != fiber.StatusOK || r.etag != `"2"` || r.body["revision"] != 2.0 {
		t.Fatalf("got %d %q %v", r.status, r.etag, r.body)
	}
	if _, ok := r.body["derived"]; ok {
		t.Error("rascunho com derived")
	}
	if name := e.do(t, http.MethodGet, "/characters", e.alice, "", "").list[0]["name"]; name != "Grok" {
		t.Errorf("nome do personagem = %v", name)
	}

	if r := e.do(t, http.MethodPut, path, e.alice, `"1"`, `{"data":{}}`); r.status != fiber.StatusConflict || r.code() != "sheet.revisionConflict" {
		t.Errorf("revisão velha: %d %q", r.status, r.code())
	}
	if r := e.do(t, http.MethodPut, path, e.bob, `"2"`, `{"data":{}}`); r.status != fiber.StatusNotFound {
		t.Errorf("ficha de outro usuário: %d", r.status)
	}
}

func TestFinalizeAndEditReady(t *testing.T) {
	e := newCharEnv(t)
	_, sheetID := e.create(t, e.alice, `{"points":3}`)
	path := "/sheets/" + sheetID

	r := e.do(t, http.MethodPost, path+"/finalize", e.alice, "", "")
	if r.status != fiber.StatusUnprocessableEntity || r.code() != "sheet.rulesViolated" {
		t.Fatalf("strict: %d %q", r.status, r.code())
	}
	e.do(t, http.MethodPut, path, e.alice, `"1"`, `{"data":{"name":"Grok","points":3}}`)

	if r := e.do(t, http.MethodPost, path+"/finalize", e.alice, `"1"`, ""); r.status != fiber.StatusConflict {
		t.Errorf("finalize com If-Match velho: %d", r.status)
	}
	r = e.do(t, http.MethodPost, path+"/finalize", e.alice, `"2"`, "")
	if r.status != fiber.StatusOK || r.body["status"] != "ready" || r.body["revision"] != 3.0 || r.body["finalizedAt"] == nil {
		t.Fatalf("finalize: %d %v", r.status, r.body)
	}
	if r.body["derived"].(map[string]any)["double"] != 6.0 {
		t.Errorf("derived = %v", r.body["derived"])
	}
	if _, ok := r.body["summary"].(map[string]any)["progress"]; ok {
		t.Error("ficha pronta com progress")
	}
	if again := e.do(t, http.MethodPost, path+"/finalize", e.alice, "", ""); again.status != fiber.StatusOK || again.body["revision"] != 3.0 {
		t.Errorf("finalize repetido: %d %v", again.status, again.body)
	}

	r = e.do(t, http.MethodPut, path, e.alice, `"3"`, `{"data":{"points":5}}`)
	if r.status != fiber.StatusOK || r.body["status"] != "ready" {
		t.Fatalf("edição da pronta: %d %v", r.status, r.body)
	}
	if w, _ := r.body["warnings"].([]any); len(w) != 1 {
		t.Errorf("warnings = %v", r.body["warnings"])
	}
	if r.body["derived"].(map[string]any)["double"] != 10.0 {
		t.Errorf("derived não foi recalculado: %v", r.body["derived"])
	}
	if w, _ := e.do(t, http.MethodGet, path, e.alice, "", "").body["warnings"].([]any); len(w) != 1 {
		t.Errorf("GET da pronta sem warnings")
	}
}

func TestDeleteCharacter(t *testing.T) {
	e := newCharEnv(t)
	characterID, sheetID := e.create(t, e.alice, `{}`)

	if r := e.do(t, http.MethodDelete, "/characters/"+characterID, e.bob, "", ""); r.status != fiber.StatusNotFound || r.code() != "character.notFound" {
		t.Errorf("personagem de outro usuário: %d %q", r.status, r.code())
	}
	if r := e.do(t, http.MethodDelete, "/characters/"+characterID, e.alice, "", ""); r.status != fiber.StatusNoContent {
		t.Fatalf("delete: %d", r.status)
	}
	if r := e.do(t, http.MethodGet, "/sheets/"+sheetID, e.alice, "", ""); r.status != fiber.StatusNotFound {
		t.Errorf("ficha sobreviveu: %d", r.status)
	}
}

func TestMigrateOnRead(t *testing.T) {
	e := newCharEnv(t, toyV2{toy{version: "1"}})
	_, sheetID := e.create(t, e.alice, `{"points":3}`)
	id, _ := primitive.ObjectIDFromHex(sheetID)
	s := e.repo.sheets[id]
	s.SchemaVersion = 1
	e.repo.sheets[id] = s

	r := e.do(t, http.MethodGet, "/sheets/"+sheetID, e.alice, "", "")
	if r.body["schemaVersion"] != 2.0 || r.body["data"].(map[string]any)["pts"] != 3.0 {
		t.Errorf("não migrou: %v", r.body)
	}
}

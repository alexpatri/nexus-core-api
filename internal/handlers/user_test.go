package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"rpg-nexus/api/core/internal/auth"
	"rpg-nexus/api/core/internal/models"
	"rpg-nexus/api/core/internal/repository"

	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type fakeUsers struct {
	mu   sync.Mutex
	byID map[primitive.ObjectID]models.User
}

func (f *fakeUsers) Insert(_ context.Context, u models.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, other := range f.byID {
		if other.Email == u.Email {
			return repository.ErrDuplicate
		}
	}
	f.byID[u.ID] = u
	return nil
}

func (f *fakeUsers) FindByEmail(_ context.Context, email string) (models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.byID {
		if u.Email == email {
			return u, nil
		}
	}
	return models.User{}, repository.ErrNotFound
}

func (f *fakeUsers) FindByID(_ context.Context, id primitive.ObjectID) (models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return models.User{}, repository.ErrNotFound
	}
	return u, nil
}

func (f *fakeUsers) UpdateName(_ context.Context, id primitive.ObjectID, name string) (models.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return models.User{}, repository.ErrNotFound
	}
	u.Name = name
	f.byID[id] = u
	return u, nil
}

func (f *fakeUsers) UpdatePasswordHash(_ context.Context, id primitive.ObjectID, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	u.PasswordHash = hash
	f.byID[id] = u
	return nil
}

type fakeSessions struct {
	mu   sync.Mutex
	byID map[primitive.ObjectID]models.Session
}

func (f *fakeSessions) Insert(_ context.Context, s models.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[s.ID] = s
	return nil
}

func (f *fakeSessions) FindActive(_ context.Context, tokenHash string, now time.Time) (models.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.byID {
		if s.TokenHash == tokenHash && s.ExpiresAt.After(now) {
			return s, nil
		}
	}
	return models.Session{}, repository.ErrNotFound
}

func (f *fakeSessions) Touch(_ context.Context, id primitive.ObjectID, now, expiresAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.byID[id]
	s.LastUsedAt, s.ExpiresAt = now, expiresAt
	f.byID[id] = s
	return nil
}

func (f *fakeSessions) Delete(_ context.Context, id primitive.ObjectID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.byID, id)
	return nil
}

func (f *fakeSessions) DeleteByUser(_ context.Context, userID primitive.ObjectID) error {
	return f.DeleteByUserExcept(context.Background(), userID, primitive.NilObjectID)
}

func (f *fakeSessions) DeleteByUserExcept(_ context.Context, userID, keep primitive.ObjectID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, s := range f.byID {
		if s.UserID == userID && id != keep {
			delete(f.byID, id)
		}
	}
	return nil
}

const ttl = 90 * 24 * time.Hour

func newTestApp() (*fiber.App, *fakeSessions) {
	users := &fakeUsers{byID: map[primitive.ObjectID]models.User{}}
	sessions := &fakeSessions{byID: map[primitive.ObjectID]models.Session{}}
	svc := NewUserHandler(users, sessions, ttl)

	app := fiber.New()
	app.Post("/auth/register", svc.RegisterHandler)
	app.Post("/auth/login", svc.LoginHandler)
	private := app.Group("", auth.Middleware(sessions, ttl))
	private.Post("/auth/logout", svc.LogoutHandler)
	private.Post("/auth/logout-all", svc.LogoutAllHandler)
	private.Get("/me", svc.MeHandler)
	private.Patch("/me", svc.UpdateMeHandler)
	private.Put("/me/password", svc.ChangePasswordHandler)
	return app, sessions
}

type result struct {
	status int
	body   map[string]any
}

func do(t *testing.T, app *fiber.App, method, path, token, body string) result {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)
	return result{status: resp.StatusCode, body: parsed}
}

func errCode(r result) string {
	e, _ := r.body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func register(t *testing.T, app *fiber.App, email string) string {
	t.Helper()
	r := do(t, app, http.MethodPost, "/auth/register", "",
		`{"name":"Alex","email":"`+email+`","password":"senha-forte"}`)
	if r.status != fiber.StatusCreated {
		t.Fatalf("register: status %d, corpo %v", r.status, r.body)
	}
	return r.body["token"].(string)
}

func TestRegister(t *testing.T) {
	app, _ := newTestApp()

	r := do(t, app, http.MethodPost, "/auth/register", "",
		`{"name":" Alex ","email":" Alex@Exemplo.com ","password":"senha-forte"}`)
	if r.status != fiber.StatusCreated {
		t.Fatalf("status %d, corpo %v", r.status, r.body)
	}
	if token, _ := r.body["token"].(string); !strings.HasPrefix(token, "nxs_") {
		t.Errorf("token sem prefixo: %q", token)
	}
	user := r.body["user"].(map[string]any)
	if user["email"] != "alex@exemplo.com" || user["name"] != "Alex" {
		t.Errorf("user = %v", user)
	}
	if _, ok := user["passwordHash"]; ok {
		t.Error("resposta expõe o hash da senha")
	}

	cases := []struct {
		name, body, code string
		status           int
		field            string
	}{
		{"e-mail repetido", `{"name":"Outro","email":"ALEX@exemplo.com","password":"senha-forte"}`, "auth.emailTaken", 409, ""},
		{"senha curta", `{"name":"Alex","email":"b@exemplo.com","password":"curta"}`, "auth.weakPassword", 400, ""},
		{"senha longa", `{"name":"Alex","email":"b@exemplo.com","password":"` + strings.Repeat("a", 73) + `"}`, "auth.weakPassword", 400, ""},
		{"sem nome", `{"name":"  ","email":"b@exemplo.com","password":"senha-forte"}`, "auth.invalidInput", 400, "name"},
		{"e-mail inválido", `{"name":"Alex","email":"não é e-mail","password":"senha-forte"}`, "auth.invalidInput", 400, "email"},
		{"tipo errado", `{"name":1,"email":"b@exemplo.com","password":"senha-forte"}`, "auth.invalidInput", 400, "name"},
		{"não é JSON", `nada`, "auth.invalidInput", 400, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := do(t, app, http.MethodPost, "/auth/register", "", tc.body)
			if r.status != tc.status || errCode(r) != tc.code {
				t.Fatalf("got %d %q, want %d %q", r.status, errCode(r), tc.status, tc.code)
			}
			params, _ := r.body["error"].(map[string]any)["params"].(map[string]any)
			if field, _ := params["field"].(string); field != tc.field {
				t.Errorf("field = %q, want %q", field, tc.field)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	app, _ := newTestApp()
	register(t, app, "alex@exemplo.com")

	r := do(t, app, http.MethodPost, "/auth/login", "", `{"email":"ALEX@exemplo.com ","password":"senha-forte"}`)
	if r.status != fiber.StatusOK {
		t.Fatalf("status %d, corpo %v", r.status, r.body)
	}
	if do(t, app, http.MethodGet, "/me", r.body["token"].(string), "").status != fiber.StatusOK {
		t.Error("token do login não abre /me")
	}

	for _, body := range []string{
		`{"email":"alex@exemplo.com","password":"errada-errada"}`,
		`{"email":"ninguem@exemplo.com","password":"senha-forte"}`,
	} {
		r := do(t, app, http.MethodPost, "/auth/login", "", body)
		if r.status != fiber.StatusUnauthorized || errCode(r) != "auth.invalidCredentials" {
			t.Errorf("%s: got %d %q", body, r.status, errCode(r))
		}
	}
}

func TestMiddleware(t *testing.T) {
	app, sessions := newTestApp()
	token := register(t, app, "alex@exemplo.com")

	for _, header := range []string{"", "nxs_desconhecido"} {
		r := do(t, app, http.MethodGet, "/me", header, "")
		if r.status != fiber.StatusUnauthorized || errCode(r) != "auth.unauthenticated" {
			t.Errorf("token %q: got %d %q", header, r.status, errCode(r))
		}
	}

	var id primitive.ObjectID
	for id = range sessions.byID {
	}
	s := sessions.byID[id]
	s.LastUsedAt = time.Now().Add(-48 * time.Hour)
	s.ExpiresAt = time.Now().Add(time.Hour)
	sessions.byID[id] = s

	if r := do(t, app, http.MethodGet, "/me", token, ""); r.status != fiber.StatusOK {
		t.Fatalf("status %d", r.status)
	}
	if got := sessions.byID[id].ExpiresAt; time.Until(got) < ttl-time.Minute {
		t.Errorf("sessão não foi renovada: expira em %v", time.Until(got))
	}

	s = sessions.byID[id]
	s.ExpiresAt = time.Now().Add(-time.Second)
	sessions.byID[id] = s
	if r := do(t, app, http.MethodGet, "/me", token, ""); r.status != fiber.StatusUnauthorized {
		t.Errorf("sessão vencida: status %d", r.status)
	}
}

func TestLogout(t *testing.T) {
	app, _ := newTestApp()
	a := register(t, app, "alex@exemplo.com")
	login := func() string {
		return do(t, app, http.MethodPost, "/auth/login", "", `{"email":"alex@exemplo.com","password":"senha-forte"}`).body["token"].(string)
	}
	b, c := login(), login()

	if r := do(t, app, http.MethodPost, "/auth/logout", a, ""); r.status != fiber.StatusNoContent {
		t.Fatalf("logout: status %d", r.status)
	}
	if do(t, app, http.MethodGet, "/me", a, "").status != fiber.StatusUnauthorized {
		t.Error("sessão continua depois do logout")
	}
	if do(t, app, http.MethodGet, "/me", b, "").status != fiber.StatusOK {
		t.Error("logout derrubou outra sessão")
	}

	if r := do(t, app, http.MethodPost, "/auth/logout-all", b, ""); r.status != fiber.StatusNoContent {
		t.Fatalf("logout-all: status %d", r.status)
	}
	for _, token := range []string{b, c} {
		if do(t, app, http.MethodGet, "/me", token, "").status != fiber.StatusUnauthorized {
			t.Error("sessão continua depois do logout-all")
		}
	}
}

func TestUpdateMe(t *testing.T) {
	app, _ := newTestApp()
	token := register(t, app, "alex@exemplo.com")

	r := do(t, app, http.MethodPatch, "/me", token, `{"name":" Alexsandro "}`)
	if r.status != fiber.StatusOK || r.body["name"] != "Alexsandro" {
		t.Fatalf("got %d %v", r.status, r.body)
	}
	if r := do(t, app, http.MethodPatch, "/me", token, `{"name":""}`); errCode(r) != "auth.invalidInput" {
		t.Errorf("nome vazio: got %d %q", r.status, errCode(r))
	}
}

func TestChangePassword(t *testing.T) {
	app, _ := newTestApp()
	current := register(t, app, "alex@exemplo.com")
	other := do(t, app, http.MethodPost, "/auth/login", "", `{"email":"alex@exemplo.com","password":"senha-forte"}`).body["token"].(string)

	r := do(t, app, http.MethodPut, "/me/password", current, `{"currentPassword":"errada-errada","newPassword":"senha-nova"}`)
	if r.status != fiber.StatusForbidden || errCode(r) != "auth.wrongPassword" {
		t.Fatalf("senha atual errada: got %d %q", r.status, errCode(r))
	}
	r = do(t, app, http.MethodPut, "/me/password", current, `{"currentPassword":"senha-forte","newPassword":"curta"}`)
	if errCode(r) != "auth.weakPassword" {
		t.Fatalf("senha nova curta: got %d %q", r.status, errCode(r))
	}

	r = do(t, app, http.MethodPut, "/me/password", current, `{"currentPassword":"senha-forte","newPassword":"senha-nova"}`)
	if r.status != fiber.StatusNoContent {
		t.Fatalf("status %d, corpo %v", r.status, r.body)
	}
	if do(t, app, http.MethodGet, "/me", current, "").status != fiber.StatusOK {
		t.Error("a sessão atual caiu")
	}
	if do(t, app, http.MethodGet, "/me", other, "").status != fiber.StatusUnauthorized {
		t.Error("a outra sessão continua")
	}
	if r := do(t, app, http.MethodPost, "/auth/login", "", `{"email":"alex@exemplo.com","password":"senha-nova"}`); r.status != fiber.StatusOK {
		t.Errorf("login com a senha nova: status %d", r.status)
	}
}

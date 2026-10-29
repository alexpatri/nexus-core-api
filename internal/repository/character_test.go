package repository

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"rpg-nexus/api/core/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Precisa de um Mongo em replica set, como o do docker-compose:
// NEXUS_TEST_MONGO_URI=mongodb://localhost:27017 go test ./internal/repository
func testDB(t *testing.T) *mongo.Database {
	t.Helper()
	uri := os.Getenv("NEXUS_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("NEXUS_TEST_MONGO_URI não definida")
	}
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	db := client.Database("test_" + primitive.NewObjectID().Hex())
	t.Cleanup(func() {
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	return db
}

func newPair(owner primitive.ObjectID, data string) (models.Character, models.Sheet) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	c := models.Character{
		ID: primitive.NewObjectID(), OwnerID: owner, System: "shadowrun", SystemVersion: "5e",
		Name: "Grok", CreatedAt: now, UpdatedAt: now,
	}
	s := models.Sheet{
		ID: primitive.NewObjectID(), CharacterID: c.ID, OwnerID: owner, System: "shadowrun", SystemVersion: "5e",
		SchemaVersion: 1, Status: models.StatusDraft, Revision: 1,
		Summary: json.RawMessage(`{"title":"Grok","subtitle":"Troll"}`), Data: json.RawMessage(data),
		CreatedAt: now, UpdatedAt: now,
	}
	return c, s
}

func TestCharacterRepository(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	r := NewCharacter(db)
	if err := r.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.EnsureIndexes(ctx); err != nil {
		t.Fatalf("boot repetido: %v", err)
	}

	owner := primitive.NewObjectID()
	c, s := newPair(owner, `{"attributes":{"bod":{"points":4}},"essence":5750,"note":"ç","tags":[1,2.5]}`)
	if err := r.Create(ctx, c, s); err != nil {
		t.Fatal(err)
	}

	got, err := r.FindSheet(ctx, owner, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	var data, want any
	_ = json.Unmarshal(got.Data, &data)
	_ = json.Unmarshal(s.Data, &want)
	if !jsonEqual(data, want) {
		t.Errorf("data = %s", got.Data)
	}
	if !strings.Contains(string(got.Data), `"essence":5750,`) {
		t.Errorf("número não voltou como JSON comum: %s", got.Data)
	}
	if got.Derived != nil || got.CampaignID != nil {
		t.Errorf("derived %s, campaignId %v", got.Derived, got.CampaignID)
	}
	var raw bson.M
	_ = db.Collection("sheets").FindOne(ctx, bson.M{"_id": s.ID}).Decode(&raw)
	if v, ok := raw["campaignId"]; !ok || v != nil {
		t.Errorf("campaignId gravado como %v (presente: %v)", v, ok)
	}
	if _, ok := raw["data"].(bson.M); !ok {
		t.Errorf("data não foi gravado como documento: %T", raw["data"])
	}

	if _, err := r.FindSheet(ctx, primitive.NewObjectID(), s.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("ficha de outro dono: %v", err)
	}

	got.Data = json.RawMessage(`{"metatype":"troll"}`)
	got.Derived = json.RawMessage(`{"limits":{"physical":10}}`)
	got.Status = models.StatusReady
	got.UpdatedAt = time.Now().UTC()
	if err := r.UpdateSheet(ctx, got, 1, "Grok, o Troll"); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateSheet(ctx, got, 1, "x"); !errors.Is(err, ErrConflict) {
		t.Errorf("revisão velha: %v", err)
	}
	got.OwnerID = primitive.NewObjectID()
	if err := r.UpdateSheet(ctx, got, 2, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("outro dono: %v", err)
	}

	items, err := r.List(ctx, owner, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "Grok, o Troll" || items[0].Sheet == nil ||
		items[0].Sheet.Revision != 2 || items[0].Sheet.Status != models.StatusReady {
		t.Fatalf("List = %+v", items)
	}
	if items, _ := r.List(ctx, owner, "dnd"); len(items) != 0 {
		t.Errorf("filtro por sistema: %+v", items)
	}

	_, other := newPair(owner, `{}`)
	other.CharacterID = c.ID
	if err := r.Create(ctx, models.Character{ID: primitive.NewObjectID(), OwnerID: owner, System: "shadowrun",
		SystemVersion: "5e", CreatedAt: other.CreatedAt, UpdatedAt: other.UpdatedAt}, other); !errors.Is(err, ErrDuplicate) {
		t.Errorf("segunda ficha-base: %v", err)
	}
	if n, _ := db.Collection("characters").CountDocuments(ctx, bson.M{}); n != 1 {
		t.Errorf("transação não desfez o personagem: %d personagens", n)
	}

	if err := r.Delete(ctx, primitive.NewObjectID(), c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("apagar de outro dono: %v", err)
	}
	if err := r.Delete(ctx, owner, c.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := db.Collection("sheets").CountDocuments(ctx, bson.M{}); n != 0 {
		t.Errorf("fichas sobraram: %d", n)
	}
}

func TestSheetValidator(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	if err := NewCharacter(db).EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := db.Collection("sheets").InsertOne(ctx, bson.M{"status": "pronta", "data": "x"})
	if err == nil {
		t.Error("validador aceitou ficha sem envelope")
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"rpg-nexus/api/core/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrConflict = errors.New("repository: revision conflict")

type characterDoc struct {
	ID            primitive.ObjectID `bson:"_id"`
	OwnerID       primitive.ObjectID `bson:"ownerId"`
	System        string             `bson:"system"`
	SystemVersion string             `bson:"systemVersion"`
	Name          string             `bson:"name"`
	CreatedAt     time.Time          `bson:"createdAt"`
	UpdatedAt     time.Time          `bson:"updatedAt"`
}

// sheetDoc guarda data, derived e summary como documentos BSON, e não como
// blob, para que fiquem legíveis e consultáveis no Mongo.
type sheetDoc struct {
	ID            primitive.ObjectID  `bson:"_id"`
	CharacterID   primitive.ObjectID  `bson:"characterId"`
	OwnerID       primitive.ObjectID  `bson:"ownerId"`
	CampaignID    *primitive.ObjectID `bson:"campaignId"`
	System        string              `bson:"system"`
	SystemVersion string              `bson:"systemVersion"`
	SchemaVersion int                 `bson:"schemaVersion"`
	Status        string              `bson:"status"`
	Revision      int                 `bson:"revision"`
	Summary       bson.Raw            `bson:"summary"`
	Data          bson.Raw            `bson:"data"`
	// nil no rascunho, gravado como null.
	Derived     *bson.Raw  `bson:"derived"`
	CreatedAt   time.Time  `bson:"createdAt"`
	UpdatedAt   time.Time  `bson:"updatedAt"`
	FinalizedAt *time.Time `bson:"finalizedAt,omitempty"`
}

type Character struct {
	client     *mongo.Client
	characters *mongo.Collection
	sheets     *mongo.Collection
}

func NewCharacter(db *mongo.Database) *Character {
	return &Character{
		client:     db.Client(),
		characters: db.Collection("characters"),
		sheets:     db.Collection("sheets"),
	}
}

func (r *Character) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := ensureValidator(ctx, r.characters.Database(), "characters", characterSchema); err != nil {
		return err
	}
	if err := ensureValidator(ctx, r.sheets.Database(), "sheets", sheetSchema); err != nil {
		return err
	}

	_, err := r.characters.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "ownerId", Value: 1}, {Key: "updatedAt", Value: -1}},
	})
	if err != nil {
		return err
	}
	_, err = r.sheets.Indexes().CreateMany(ctx, []mongo.IndexModel{
		// campaignId é gravado como null na ficha-base, e o null também entra na unicidade.
		{Keys: bson.D{{Key: "characterId", Value: 1}, {Key: "campaignId", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "ownerId", Value: 1}, {Key: "updatedAt", Value: -1}}},
		{Keys: bson.D{{Key: "campaignId", Value: 1}}},
	})
	return err
}

func (r *Character) Create(ctx context.Context, c models.Character, s models.Sheet) error {
	sd, err := toSheetDoc(s)
	if err != nil {
		return err
	}
	return r.transaction(ctx, func(ctx mongo.SessionContext) error {
		if _, err := r.characters.InsertOne(ctx, toCharacterDoc(c)); err != nil {
			return err
		}
		_, err := r.sheets.InsertOne(ctx, sd)
		return err
	})
}

func (r *Character) List(ctx context.Context, owner primitive.ObjectID, system string) ([]models.CharacterListItem, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	filter := bson.M{"ownerId": owner}
	if system != "" {
		filter["system"] = system
	}
	cur, err := r.characters.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}))
	if err != nil {
		return nil, err
	}
	var chars []characterDoc
	if err := cur.All(ctx, &chars); err != nil {
		return nil, err
	}

	ids := make([]primitive.ObjectID, len(chars))
	for i, c := range chars {
		ids[i] = c.ID
	}
	cur, err = r.sheets.Find(ctx,
		bson.M{"ownerId": owner, "characterId": bson.M{"$in": ids}, "campaignId": nil},
		options.Find().SetProjection(bson.M{"data": 0, "derived": 0}),
	)
	if err != nil {
		return nil, err
	}
	var sheets []sheetDoc
	if err := cur.All(ctx, &sheets); err != nil {
		return nil, err
	}
	byCharacter := make(map[primitive.ObjectID]*models.SheetSummary, len(sheets))
	for _, sd := range sheets {
		summary, err := toJSON(sd.Summary)
		if err != nil {
			return nil, err
		}
		byCharacter[sd.CharacterID] = &models.SheetSummary{
			ID:        sd.ID,
			Status:    sd.Status,
			Revision:  sd.Revision,
			Summary:   summary,
			UpdatedAt: sd.UpdatedAt,
		}
	}

	items := make([]models.CharacterListItem, len(chars))
	for i, c := range chars {
		items[i] = models.CharacterListItem{Character: fromCharacterDoc(c), Sheet: byCharacter[c.ID]}
	}
	return items, nil
}

func (r *Character) FindSheet(ctx context.Context, owner, id primitive.ObjectID) (models.Sheet, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var sd sheetDoc
	if err := r.sheets.FindOne(ctx, bson.M{"_id": id, "ownerId": owner}).Decode(&sd); err != nil {
		return models.Sheet{}, translateErr(err)
	}
	return fromSheetDoc(sd)
}

// UpdateSheet grava o estado de s se a revisão no banco ainda for expected, e
// leva o nome do personagem junto, porque ele sai do resumo da ficha.
func (r *Character) UpdateSheet(ctx context.Context, s models.Sheet, expected int, name string) error {
	sd, err := toSheetDoc(s)
	if err != nil {
		return err
	}
	set := bson.M{
		"schemaVersion": sd.SchemaVersion,
		"status":        sd.Status,
		"summary":       sd.Summary,
		"data":          sd.Data,
		"derived":       sd.Derived,
		"updatedAt":     sd.UpdatedAt,
	}
	if sd.FinalizedAt != nil {
		set["finalizedAt"] = sd.FinalizedAt
	}

	return r.transaction(ctx, func(ctx mongo.SessionContext) error {
		res, err := r.sheets.UpdateOne(ctx,
			bson.M{"_id": sd.ID, "ownerId": sd.OwnerID, "revision": expected},
			bson.M{"$set": set, "$inc": bson.M{"revision": 1}},
		)
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			n, err := r.sheets.CountDocuments(ctx, bson.M{"_id": sd.ID, "ownerId": sd.OwnerID})
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrNotFound
			}
			return ErrConflict
		}
		if sd.CampaignID != nil {
			return nil
		}
		_, err = r.characters.UpdateByID(ctx, sd.CharacterID, bson.M{"$set": bson.M{
			"name":      name,
			"updatedAt": sd.UpdatedAt,
		}})
		return err
	})
}

func (r *Character) Delete(ctx context.Context, owner, id primitive.ObjectID) error {
	return r.transaction(ctx, func(ctx mongo.SessionContext) error {
		res, err := r.characters.DeleteOne(ctx, bson.M{"_id": id, "ownerId": owner})
		if err != nil {
			return err
		}
		if res.DeletedCount == 0 {
			return ErrNotFound
		}
		_, err = r.sheets.DeleteMany(ctx, bson.M{"characterId": id, "ownerId": owner})
		return err
	})
}

func (r *Character) transaction(ctx context.Context, fn func(mongo.SessionContext) error) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	sess, err := r.client.StartSession()
	if err != nil {
		return err
	}
	defer sess.EndSession(ctx)

	_, err = sess.WithTransaction(ctx, func(ctx mongo.SessionContext) (any, error) {
		return nil, fn(ctx)
	})
	return translateErr(err)
}

func toCharacterDoc(c models.Character) characterDoc {
	return characterDoc{
		ID:            c.ID,
		OwnerID:       c.OwnerID,
		System:        c.System,
		SystemVersion: c.SystemVersion,
		Name:          c.Name,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     c.UpdatedAt,
	}
}

func fromCharacterDoc(d characterDoc) models.Character {
	return models.Character{
		ID:            d.ID,
		OwnerID:       d.OwnerID,
		System:        d.System,
		SystemVersion: d.SystemVersion,
		Name:          d.Name,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
}

func toSheetDoc(s models.Sheet) (sheetDoc, error) {
	summary, err := toBSON(s.Summary)
	if err != nil {
		return sheetDoc{}, err
	}
	data, err := toBSON(s.Data)
	if err != nil {
		return sheetDoc{}, err
	}
	var derived *bson.Raw
	if s.Derived != nil {
		raw, err := toBSON(s.Derived)
		if err != nil {
			return sheetDoc{}, err
		}
		derived = &raw
	}
	return sheetDoc{
		ID:            s.ID,
		CharacterID:   s.CharacterID,
		OwnerID:       s.OwnerID,
		CampaignID:    s.CampaignID,
		System:        s.System,
		SystemVersion: s.SystemVersion,
		SchemaVersion: s.SchemaVersion,
		Status:        s.Status,
		Revision:      s.Revision,
		Summary:       summary,
		Data:          data,
		Derived:       derived,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
		FinalizedAt:   s.FinalizedAt,
	}, nil
}

func fromSheetDoc(d sheetDoc) (models.Sheet, error) {
	summary, err := toJSON(d.Summary)
	if err != nil {
		return models.Sheet{}, err
	}
	data, err := toJSON(d.Data)
	if err != nil {
		return models.Sheet{}, err
	}
	var derived json.RawMessage
	if d.Derived != nil {
		if derived, err = toJSON(*d.Derived); err != nil {
			return models.Sheet{}, err
		}
	}
	return models.Sheet{
		ID:            d.ID,
		CharacterID:   d.CharacterID,
		OwnerID:       d.OwnerID,
		CampaignID:    d.CampaignID,
		System:        d.System,
		SystemVersion: d.SystemVersion,
		SchemaVersion: d.SchemaVersion,
		Status:        d.Status,
		Revision:      d.Revision,
		Summary:       summary,
		Data:          data,
		Derived:       derived,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
		FinalizedAt:   d.FinalizedAt,
	}, nil
}

func toBSON(j json.RawMessage) (bson.Raw, error) {
	var raw bson.Raw
	err := bson.UnmarshalExtJSON(j, false, &raw)
	return raw, err
}

// Modo relaxado: números saem como números JSON comuns, não como {"$numberInt": …}.
func toJSON(raw bson.Raw) (json.RawMessage, error) {
	return bson.MarshalExtJSON(raw, false, false)
}

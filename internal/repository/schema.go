package repository

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// O validador cobre só o envelope; o conteúdo de data é da estratégia do sistema.
var sheetSchema = bson.M{
	"bsonType": "object",
	"required": bson.A{"characterId", "ownerId", "campaignId", "system", "systemVersion",
		"schemaVersion", "status", "revision", "data", "summary", "createdAt", "updatedAt"},
	"properties": bson.M{
		"campaignId":    bson.M{"bsonType": bson.A{"objectId", "null"}},
		"status":        bson.M{"enum": bson.A{"draft", "ready"}},
		"revision":      bson.M{"bsonType": bson.A{"int", "long"}, "minimum": 1},
		"schemaVersion": bson.M{"bsonType": bson.A{"int", "long"}, "minimum": 1},
		"data":          bson.M{"bsonType": "object"},
		"summary":       bson.M{"bsonType": "object"},
		"derived":       bson.M{"bsonType": bson.A{"object", "null"}},
	},
}

var characterSchema = bson.M{
	"bsonType": "object",
	"required": bson.A{"ownerId", "system", "systemVersion", "name", "createdAt", "updatedAt"},
	"properties": bson.M{
		"ownerId":       bson.M{"bsonType": "objectId"},
		"system":        bson.M{"bsonType": "string"},
		"systemVersion": bson.M{"bsonType": "string"},
		"name":          bson.M{"bsonType": "string"},
	},
}

const namespaceExists = 48

// ensureValidator cria a collection com o validador ou, se ela já existe,
// troca o validador, para o boot ser idempotente.
func ensureValidator(ctx context.Context, db *mongo.Database, name string, schema bson.M) error {
	validator := bson.M{"$jsonSchema": schema}
	err := db.CreateCollection(ctx, name, options.CreateCollection().SetValidator(validator))
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) && cmdErr.Code == namespaceExists {
		return db.RunCommand(ctx, bson.D{{Key: "collMod", Value: name}, {Key: "validator", Value: validator}}).Err()
	}
	return err
}

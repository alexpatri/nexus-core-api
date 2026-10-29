package repository

import (
	"context"
	"time"

	"rpg-nexus/api/core/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Session struct {
	coll *mongo.Collection
}

func NewSession(db *mongo.Database) *Session {
	return &Session{coll: db.Collection("sessions")}
}

func (r *Session) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := r.coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "userId", Value: 1}}},
		{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
	})
	return err
}

func (r *Session) Insert(ctx context.Context, s models.Session) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := r.coll.InsertOne(ctx, s)
	return translateErr(err)
}

// O TTL do Mongo roda a cada 60 s, então a validade também é filtrada aqui.
func (r *Session) FindActive(ctx context.Context, tokenHash string, now time.Time) (models.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var s models.Session
	err := r.coll.FindOne(ctx, bson.M{
		"tokenHash": tokenHash,
		"expiresAt": bson.M{"$gt": now},
	}).Decode(&s)
	return s, translateErr(err)
}

func (r *Session) Touch(ctx context.Context, id primitive.ObjectID, now, expiresAt time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := r.coll.UpdateByID(ctx, id, bson.M{"$set": bson.M{
		"lastUsedAt": now,
		"expiresAt":  expiresAt,
	}})
	return err
}

func (r *Session) Delete(ctx context.Context, id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *Session) DeleteByUser(ctx context.Context, userID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := r.coll.DeleteMany(ctx, bson.M{"userId": userID})
	return err
}

func (r *Session) DeleteByUserExcept(ctx context.Context, userID, keep primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := r.coll.DeleteMany(ctx, bson.M{"userId": userID, "_id": bson.M{"$ne": keep}})
	return err
}

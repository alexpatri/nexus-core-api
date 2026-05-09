package repository

import (
	"context"
	"time"

	"rpg-nexus/api/core/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Character struct {
	coll *mongo.Collection
}

func NewCharacter(db *mongo.Database) *Character {
	return &Character{coll: db.Collection("character")}
}

func (r *Character) Find(ctx context.Context, ownerID primitive.ObjectID) (models.Characters, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cur, err := r.coll.Find(ctx, bson.M{"ownerId": ownerID})
	if err != nil {
		return models.Characters{}, err
	}
	defer cur.Close(ctx)

	var out models.Characters
	if err := cur.All(ctx, &out.Docs); err != nil {
		return models.Characters{}, err
	}
	return out, nil
}

func (r *Character) FindByID(ctx context.Context, id string, ownerID primitive.ObjectID) (models.Character, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.Character{}, ErrNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var out models.Character
	err = r.coll.FindOne(ctx, bson.M{"_id": objID, "ownerId": ownerID}).Decode(&out)
	return out, translateErr(err)
}

func (r *Character) Insert(ctx context.Context, c models.Character) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := r.coll.InsertOne(ctx, c)
	return err
}

func (r *Character) UpdateByID(ctx context.Context, id string, ownerID primitive.ObjectID, c models.Character) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return ErrNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": objID, "ownerId": ownerID}, bson.M{"$set": c})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Character) DeleteByID(ctx context.Context, id string, ownerID primitive.ObjectID) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return ErrNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := r.coll.DeleteOne(ctx, bson.M{"_id": objID, "ownerId": ownerID})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

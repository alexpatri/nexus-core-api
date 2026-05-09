package repository

import (
	"context"
	"time"

	"rpg-nexus/api/core/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type Campaign struct {
	coll *mongo.Collection
}

func NewCampaign(db *mongo.Database) *Campaign {
	return &Campaign{coll: db.Collection("campaign")}
}

func (r *Campaign) Find(ctx context.Context, ownerID primitive.ObjectID) ([]models.Campaign, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cur, err := r.coll.Find(ctx, bson.M{"ownerId": ownerID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []models.Campaign
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Campaign) FindByID(ctx context.Context, id string) (models.Campaign, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return models.Campaign{}, ErrNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var out models.Campaign
	err = r.coll.FindOne(ctx, bson.M{"_id": objID}).Decode(&out)
	return out, translateErr(err)
}

func (r *Campaign) Insert(ctx context.Context, c models.Campaign) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := r.coll.InsertOne(ctx, c)
	return err
}

func (r *Campaign) UpdateByID(ctx context.Context, id string, c models.Campaign) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return ErrNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$set": c})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Campaign) DeleteByID(ctx context.Context, id string, ownerID primitive.ObjectID) error {
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

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

type User struct {
	coll *mongo.Collection
}

func NewUser(db *mongo.Database) *User {
	return &User{coll: db.Collection("users")}
}

func (r *User) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := r.coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

func (r *User) Insert(ctx context.Context, u models.User) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := r.coll.InsertOne(ctx, u)
	return translateErr(err)
}

func (r *User) FindByEmail(ctx context.Context, email string) (models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var u models.User
	err := r.coll.FindOne(ctx, bson.M{"email": email}).Decode(&u)
	return u, translateErr(err)
}

func (r *User) FindByID(ctx context.Context, id primitive.ObjectID) (models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var u models.User
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&u)
	return u, translateErr(err)
}

func (r *User) UpdateName(ctx context.Context, id primitive.ObjectID, name string) (models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var u models.User
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"name": name}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&u)
	return u, translateErr(err)
}

func (r *User) UpdatePasswordHash(ctx context.Context, id primitive.ObjectID, hash string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	res, err := r.coll.UpdateByID(ctx, id, bson.M{"$set": bson.M{"passwordHash": hash}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

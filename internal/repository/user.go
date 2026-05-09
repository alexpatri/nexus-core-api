package repository

import (
	"context"
	"time"

	"rpg-nexus/api/core/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type User struct {
	coll *mongo.Collection
}

func NewUser(db *mongo.Database) *User {
	return &User{coll: db.Collection("user")}
}

func (r *User) Insert(ctx context.Context, u models.User) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := r.coll.InsertOne(ctx, u)
	return err
}

func (r *User) FindByEmail(ctx context.Context, email string) (models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var u models.User
	err := r.coll.FindOne(ctx, bson.M{"email": email}).Decode(&u)
	return u, translateErr(err)
}

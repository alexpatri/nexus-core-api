package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Session struct {
	ID         primitive.ObjectID `bson:"_id"`
	UserID     primitive.ObjectID `bson:"userId"`
	TokenHash  string             `bson:"tokenHash"`
	DeviceName string             `bson:"deviceName,omitempty"`
	CreatedAt  time.Time          `bson:"createdAt"`
	LastUsedAt time.Time          `bson:"lastUsedAt"`
	ExpiresAt  time.Time          `bson:"expiresAt"`
}

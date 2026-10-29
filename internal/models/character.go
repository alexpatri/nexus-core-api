package models

import (
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	StatusDraft = "draft"
	StatusReady = "ready"
)

type Character struct {
	ID            primitive.ObjectID `json:"id"`
	OwnerID       primitive.ObjectID `json:"-"`
	System        string             `json:"system"`
	SystemVersion string             `json:"systemVersion"`
	Name          string             `json:"name"`
	CreatedAt     time.Time          `json:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt"`
}

type Sheet struct {
	ID            primitive.ObjectID  `json:"id"`
	CharacterID   primitive.ObjectID  `json:"characterId"`
	OwnerID       primitive.ObjectID  `json:"-"`
	CampaignID    *primitive.ObjectID `json:"campaignId"`
	System        string              `json:"system"`
	SystemVersion string              `json:"systemVersion"`
	SchemaVersion int                 `json:"schemaVersion"`
	Status        string              `json:"status"`
	Revision      int                 `json:"revision"`
	Summary       json.RawMessage     `json:"summary"`
	Data          json.RawMessage     `json:"data"`
	Derived       json.RawMessage     `json:"derived,omitempty"`
	CreatedAt     time.Time           `json:"createdAt"`
	UpdatedAt     time.Time           `json:"updatedAt"`
	FinalizedAt   *time.Time          `json:"finalizedAt,omitempty"`
}

// CharacterListItem é o personagem com a ficha-base sem data nem derived.
type CharacterListItem struct {
	Character
	Sheet *SheetSummary `json:"sheet"`
}

type SheetSummary struct {
	ID        primitive.ObjectID `json:"id"`
	Status    string             `json:"status"`
	Revision  int                `json:"revision"`
	Summary   json.RawMessage    `json:"summary"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

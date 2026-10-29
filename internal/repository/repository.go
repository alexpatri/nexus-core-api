package repository

import (
	"errors"

	"go.mongodb.org/mongo-driver/mongo"
)

var (
	ErrNotFound  = errors.New("repository: not found")
	ErrDuplicate = errors.New("repository: duplicate")
)

func translateErr(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate
	}
	return err
}

package repository

import (
	"errors"

	"go.mongodb.org/mongo-driver/mongo"
)

var ErrNotFound = errors.New("repository: not found")

func translateErr(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	return err
}

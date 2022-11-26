// Package primitive wraps MongoDB-specific structures
package primitive

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func ObjectIDFromString(id string) (*primitive.ObjectID, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid object id: %w", err)
	}
	return &objID, nil
}

func ToObjectID(id interface{}) (*primitive.ObjectID, error) {
	objID, ok := id.(primitive.ObjectID)
	if !ok {
		return nil, fmt.Errorf("%v (%T) is not a valid ObjectID", id, id)
	}
	return &objID, nil
}

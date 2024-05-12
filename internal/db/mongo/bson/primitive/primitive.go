// Package primitive wraps MongoDB-specific structures.
package primitive

import (
	"fmt"
	"regexp"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func ObjectIDFromString(id string) (primitive.ObjectID, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return primitive.NilObjectID, fmt.Errorf("invalid object id: %w", err)
	}
	return objID, nil
}

func ToObjectID(id interface{}) (primitive.ObjectID, error) {
	objID, ok := id.(primitive.ObjectID)
	if !ok {
		return primitive.NilObjectID, fmt.Errorf("%v (%T) is not a valid ObjectID", id, id)
	}
	return objID, nil
}

func MatchIgnoreCaseRegex(word string) primitive.Regex {
	word = regexp.QuoteMeta(word)
	r := primitive.Regex{
		Pattern: word,
		Options: "i",
	}
	return r
}

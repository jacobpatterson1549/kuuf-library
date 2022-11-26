package mongo

import (
	"fmt"
	"strings"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func (Database) d(e ...bson.E) bson.D {
	return bson.D(e)
}

func (Database) e(key string, value interface{}) bson.E {
	return bson.E{
		Key:   key,
		Value: value,
	}
}
func (Database) a(d ...interface{}) bson.A {
	return bson.A(d)
}

func (d Database) objectID(id string) (bson.D, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid object id: %w", err)
	}
	return d.d(d.e(bookIDField, objID)), nil
}

func (d Database) objectIDFrom(id interface{}) (*primitive.ObjectID, error) {
	objID, ok := id.(primitive.ObjectID)
	if !ok {
		return nil, fmt.Errorf("%v (%T) is not a valid ObjectID", id, id)
	}
	return &objID, nil
}

func (d Database) filter(filter book.Filter) []bson.E {
	var parts []bson.E
	if len(filter.Subject) != 0 {
		subjectPart := d.e(bookSubjectField, filter.Subject)
		parts = append(parts, subjectPart)
	}
	if len(filter.HeaderParts) != 0 {
		joinedFilter := strings.Join(filter.HeaderParts, "|")
		regex := primitive.Regex{
			Pattern: joinedFilter,
			Options: "i",
		}
		headerParts := (d.e(
			"$or",
			d.a(
				d.d(d.e(bookTitleField, regex)),
				d.d(d.e(bookAuthorField, regex)),
				d.d(d.e(bookSubjectField, regex)),
			)))
		parts = append(parts, headerParts)
	}
	if len(parts) == 0 {
		parts = append(parts, d.e("", nil)) // mongo does not like a nil slice
	}
	return parts
}

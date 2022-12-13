// Package bson wraps native mongo bson objects.
package bson

import (
	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/mongo/bson/primitive"
	"go.mongodb.org/mongo-driver/bson"
)

type Filter struct {
	SubjectKey string
	HeaderKeys []string
}

func (f Filter) From(filter book.Filter) []bson.E {
	parts := make([]bson.E, 0, 2)
	if len(filter.Subject) != 0 {
		subjectPart := E(f.SubjectKey, filter.Subject)
		parts = append(parts, subjectPart)
	}
	if len(filter.HeaderPart) != 0 {
		regex := primitive.MatchIgnoreCaseRegex(filter.HeaderPart)
		headerFilters := make([]interface{}, len(f.HeaderKeys))
		for i, k := range f.HeaderKeys {
			headerFilters[i] = D(E(k, regex))
		}
		headerParts := E("$or", A(headerFilters...))
		parts = append(parts, headerParts)
	}
	if len(parts) == 0 {
		parts = append(parts, E("", nil))
	}
	return parts
}

func D(e ...bson.E) bson.D {
	return bson.D(e)
}

func E(key string, value interface{}) bson.E {
	return bson.E{
		Key:   key,
		Value: value,
	}
}
func A(d ...interface{}) bson.A {
	return bson.A(d)
}

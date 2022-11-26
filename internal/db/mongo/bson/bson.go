// Package bson wraps native mongo bson objects.
package bson

import (
	"strings"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

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

type Filter struct {
	SubjectKey string
	HeaderKeys []string
}

func (f Filter) From(filter book.Filter) []bson.E {
	var parts []bson.E
	if len(filter.Subject) != 0 {
		subjectPart := E(f.SubjectKey, filter.Subject)
		parts = append(parts, subjectPart)
	}
	if len(filter.HeaderParts) != 0 {
		joinedFilter := strings.Join(filter.HeaderParts, "|")
		regex := primitive.Regex{ // TODO: move this to kuuf-library package
			Pattern: joinedFilter,
			Options: "i",
		}
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

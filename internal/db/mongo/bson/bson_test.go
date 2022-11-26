package bson

import (
	"reflect"
	"testing"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/mongo/bson/primitive"
	"go.mongodb.org/mongo-driver/bson"
)

func TestD(t *testing.T) {
	tests := []struct {
		name string
		e    []bson.E
		want bson.D
	}{
		{"nil", nil, nil},
		{"empty", []bson.E{}, bson.D{}},
		{"single", []bson.E{{Key: "k1", Value: 1}}, bson.D{{Key: "k1", Value: 1}}},
		{"multiple", []bson.E{{Key: "k1", Value: 1}, {Key: "k2", Value: 2}, {Key: "k0", Value: "f"}}, bson.D{{Key: "k1", Value: 1}, {Key: "k2", Value: 2}, {Key: "k0", Value: "f"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if want, got := test.want, D(test.e...); !reflect.DeepEqual(want, got) {
				t.Errorf("not equal: \n wanted %v \n got:   %v", want, got)
			}
		})
	}
}

func TestE(t *testing.T) {
	date3 := time.Date(2022, 11, 26, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		key   string
		value interface{}
		want  bson.E
	}{
		{"string", "k1", "1", bson.E{Key: "k1", Value: "1"}},
		{"int", "K2", 2, bson.E{Key: "K2", Value: 2}},
		{"date", "3", date3, bson.E{Key: "3", Value: date3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if want, got := test.want, E(test.key, test.value); !reflect.DeepEqual(want, got) {
				t.Errorf("not equal: \n wanted %v \n got:   %v", want, got)
			}
		})
	}
}

func TestA(t *testing.T) {
	tests := []struct {
		name string
		vals []interface{}
		want bson.A
	}{
		{"nil", nil, nil},
		{"empty", []interface{}{}, bson.A{}},
		{"single", []interface{}{1}, bson.A{1}},
		{"multiple", []interface{}{"...", bson.D{{Key: "k1", Value: 1}, {Key: "k2", Value: 2}}, 4}, bson.A{"...", bson.D{{Key: "k1", Value: 1}, {Key: "k2", Value: 2}}, 4}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if want, got := test.want, A(test.vals...); !reflect.DeepEqual(want, got) {
				t.Errorf("not equal: \n wanted %v \n got:   %v", want, got)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	f := Filter{
		SubjectKey: "k1",
		HeaderKeys: []string{"k2", "k3"},
	}
	tests := []struct {
		name   string
		filter book.Filter
		want   []bson.E
	}{
		{
			name: "empty",
			want: []bson.E{{}}, // mongo does not like a nil slice
		},
		{
			name: "subject only",
			filter: book.Filter{
				Subject: "abc",
			},
			want: []bson.E{{Key: "k1", Value: "abc"}},
		},
		{
			name: "query only",
			filter: book.Filter{
				HeaderParts: []string{"x", "y", "z"},
			},
			want: []bson.E{{
				Key: "$or",
				Value: bson.A{
					bson.D{bson.E{Key: "k2", Value: primitive.MatchAnyIgnoreCaseRegex("x", "y", "z")}},
					bson.D{bson.E{Key: "k3", Value: primitive.MatchAnyIgnoreCaseRegex("x", "y", "z")}},
				}}},
		},
		{
			name: "full filter",
			filter: book.Filter{
				Subject:     "simple",
				HeaderParts: []string{"good"},
			},
			want: []bson.E{
				{Key: "k1", Value: "simple"},
				{
					Key: "$or",
					Value: bson.A{
						bson.D{bson.E{Key: "k2", Value: primitive.MatchAnyIgnoreCaseRegex("good")}},
						bson.D{bson.E{Key: "k3", Value: primitive.MatchAnyIgnoreCaseRegex("good")}},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if want, got := test.want, f.From(test.filter); !reflect.DeepEqual(want, got) {
				t.Errorf("not equal: \n wanted %v \n got:   %v", want, got)
			}
		})
	}
}

package csv

import (
	"reflect"
	"testing"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
)

func TestReadBookHeaders(t *testing.T) {
	titles := []string{"Apple", "Blueberry", "Cranberry", "Durian", "Eggplant"}
	books := make([]book.Book, len(titles))
	for i, t := range titles {
		books[i].Header.Title = t
	}
	tests := []struct {
		name   string
		limit  int
		offset int
		want   []book.Header
	}{
		{"zero offset", 2, 0, []book.Header{{Title: "Apple"}, {Title: "Blueberry"}}},
		{"middle", 3, 1, []book.Header{{Title: "Blueberry"}, {Title: "Cranberry"}, {Title: "Durian"}}},
		{"Last only", 3, 4, []book.Header{{Title: "Eggplant"}}},
		{"Past end", 2, 5, []book.Header{}},
		{"Past end by many", 2, 8, []book.Header{}},
		{"none", 0, 0, []book.Header{}},
		{"negative limit", -1, 0, []book.Header{}},
		{"negative offset", 0, -1, []book.Header{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				Books: books,
			}
			got, err := d.ReadBookHeaders(book.Filter{}, test.limit, test.offset)
			switch {
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("not equal: \n wanted: %v \n got:    %v", test.want, got)
			}
		})
	}
}

func TestReadBookSubjects(t *testing.T) {
	books := []book.Book{
		{Header: book.Header{Subject: "plants"}},
		{Header: book.Header{Subject: "animals"}},
		{Header: book.Header{Subject: "animals"}},
		{Header: book.Header{Subject: "plants"}},
		{Header: book.Header{Subject: "animals"}},
		{Header: book.Header{Subject: "liquids"}},
	}
	tests := []struct {
		name   string
		limit  int
		offset int
		want   []book.Subject
	}{
		{"zero offset", 2, 0, []book.Subject{{Name: "animals", Count: 3}, {Name: "plants", Count: 2}}},
		{"middle", 1, 1, []book.Subject{{Name: "plants", Count: 2}}},
		{"Last only", 3, 2, []book.Subject{{Name: "liquids", Count: 1}}},
		{"Past end", 2, 5, []book.Subject{}},
		{"none", 0, 0, []book.Subject{}},
		{"negative limit", -1, 0, []book.Subject{}},
		{"negative offset", 0, -1, []book.Subject{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				Books: books,
			}
			got, err := d.ReadBookSubjects(test.limit, test.offset)
			switch {
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("not equal: \n wanted: %v \n got:    %v", test.want, got)
			}
		})
	}
}

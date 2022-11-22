package mongo

import (
	"strings"
	"testing"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"go.mongodb.org/mongo-driver/bson"
)

func TestMBook(t *testing.T) {
	datePD := time.Date(2001, 7, 4, 0, 0, 0, 0, time.UTC)
	dateAD := time.Date(2022, 11, 16, 0, 0, 0, 0, time.UTC)
	m := mBook{
		Header: mHeader{
			ID:      "1",
			Title:   "2",
			Author:  "3",
			Subject: "4",
		},
		Description:   "5",
		DeweyDecClass: "6",
		Pages:         7,
		Publisher:     "8",
		PublishDate:   datePD,
		AddedDate:     dateAD,
		EAN_ISBN13:    "11",
		UPC_ISBN10:    "12",
	}
	b := book.Book{
		Header: book.Header{
			ID:      "1",
			Title:   "2",
			Author:  "3",
			Subject: "4",
		},
		Description:   "5",
		DeweyDecClass: "6",
		Pages:         7,
		Publisher:     "8",
		PublishDate:   datePD,
		AddedDate:     dateAD,
		EAN_ISBN13:    "11",
		UPC_ISBN10:    "12",
	}
	t.Run("mBook.Book()", func(t *testing.T) {
		if want, got := b, m.Book(); want != got {
			t.Errorf("not equal: \n wanted: %+v \n got:    %+v", want, got)
		}
	})
	t.Run("mongoBook(book.Book)", func(t *testing.T) {
		want, got := m, mongoBook(b)
		if want != got {
			t.Errorf("not equal: \n wanted: %+v \n got:    %+v", want, got)
		}
	})
	t.Run("bson Marshal new book", func(t *testing.T) {
		m := m
		m.Header.ID = ""
		b, err := bson.Marshal(m)
		if err != nil {
			t.Fatalf("marshalling mBook: %v", err)
		}
		if want, got := "subject", string(b); !strings.Contains(got, want) {
			t.Errorf("marshalled bson did not contain %q: \n %q", want, got)
		}
		if exclude, got := "_id", string(b); strings.Contains(got, exclude) {
			t.Errorf("marshalled bson did contain %q when marshalling new book: \n %q", exclude, got)
		}
	})
}

func TestMSubject(t *testing.T) {
	m := mSubject{
		Name:  "poetry",
		Count: 32,
	}
	want := book.Subject{
		Name:  "poetry",
		Count: 32,
	}
	got := m.Subject()
	if want != got {
		t.Errorf("not equal: \n wanted: %+v \n got:    %+v", want, got)
	}
}

package book

import (
	"reflect"
	"testing"
	"time"
)

func TestNewIDLength(t *testing.T) {
	id := NewID()
	if len(id) != 32 {
		t.Errorf("unwanted id length: %v", len(id))
	}
}

func TestSort(t *testing.T) {
	tests := []struct {
		name string
		s    Books
		want Books
	}{
		{
			name: "empty",
		},
		{
			name: "single",
			s:    Books{{Header: Header{Subject: "s"}}},
			want: Books{{Header: Header{Subject: "s"}}},
		},
		{
			name: "by subject, then title",
			s: Books{
				{Header: Header{ID: "7", Title: "Zoology", Author: "Boas", Subject: "Animals"}, Pages: 100},
				{Header: Header{ID: "9", Title: "Secrets", Author: "Everyone", Subject: "Behind others"}, Pages: 5},
				{Header: Header{ID: "13", Title: "Lemurs", Author: "Anonymous", Subject: "Animals"}, Pages: 400},
			},
			want: Books{
				{Header: Header{ID: "13", Title: "Lemurs", Author: "Anonymous", Subject: "Animals"}, Pages: 400},
				{Header: Header{ID: "7", Title: "Zoology", Author: "Boas", Subject: "Animals"}, Pages: 100},
				{Header: Header{ID: "9", Title: "Secrets", Author: "Everyone", Subject: "Behind others"}, Pages: 5},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.s.Sort()
			if want, got := test.want, test.s; !reflect.DeepEqual(want, got) {
				t.Errorf("not equal: \n wanted: %+v \n got:    %+v", want, got)
			}
		})
	}
}

func TestStringBookBook(t *testing.T) {
	tests := []struct {
		name       string
		dateLayout DateLayout
		sb         StringBook
		want       *Book
		wantOk     bool
	}{
		{"empty, no layout", "", StringBook{}, &Book{}, true},
		{"bad pages", HyphenatedYYYYMMDD, StringBook{Pages: "a"}, nil, false},
		{"bad publish date", HyphenatedYYYYMMDD, StringBook{PublishDate: "monday"}, nil, false},
		{"bad added date", HyphenatedYYYYMMDD, StringBook{PublishDate: "12/31/2012"}, nil, false}, // date uses wrong layout
		{"minimal", HyphenatedYYYYMMDD, StringBook{
			Title:     "a",
			Author:    "b",
			Subject:   "c",
			Pages:     "1",
			AddedDate: "2008-07-04",
		}, &Book{
			Header: Header{
				Title:   "a",
				Author:  "b",
				Subject: "c",
			},
			Pages:     1,
			AddedDate: time.Date(2008, 7, 4, 0, 0, 0, 0, time.UTC),
		}, true},
		{"all fields", SlashMMDDYYYY, StringBook{
			ID:            "secret",
			Title:         "Readings",
			Author:        "people",
			Subject:       "stuff",
			DeweyDecClass: "¿unknown?",
			Pages:         "42",
			Publisher:     "Nobody",
			PublishDate:   "06/09/2001", // note the leading zeroes in the date
			AddedDate:     "12/31/2012",
			EAN_ISBN13:    "1234567890123",
			UPC_ISBN10:    "1234567890",
			ImageBase64:   "base64_encoded",
		}, &Book{
			Header: Header{
				ID:      "secret",
				Title:   "Readings",
				Author:  "people",
				Subject: "stuff",
			},
			DeweyDecClass: "¿unknown?",
			Pages:         42,
			Publisher:     "Nobody",
			PublishDate:   time.Date(2001, 6, 9, 0, 0, 0, 0, time.UTC),
			AddedDate:     time.Date(2012, 12, 31, 0, 0, 0, 0, time.UTC),
			EAN_ISBN13:    "1234567890123",
			UPC_ISBN10:    "1234567890",
			ImageBase64:   "base64_encoded",
		}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.sb.Book(test.dateLayout)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case *test.want != *got:
				t.Errorf("not equal: \n wanted: %+v \n got:    %+v", test.want, got)
			}
		})
	}
}

package csv

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
)

func TestNewDatabase(t *testing.T) {
	happyPathSortedBooks := make(book.Books, len(exampleCSV.books))
	copy(happyPathSortedBooks, exampleCSV.books)
	happyPathSortedBooks.Sort()
	tests := []struct {
		name   string
		csv    string
		wantOk bool
		want   *Database
	}{
		{
			name:   "empty csv",
			wantOk: true,
			want:   &Database{Books: []book.Book{}},
		},
		{
			name:   "default library.csv (no books)",
			csv:    header,
			wantOk: true,
			want:   &Database{Books: []book.Book{}},
		},
		{
			name: "bad csv",
			csv:  "header,with,four,columns" + "\n" + "only,3,columns",
		},
		{
			name: "bad header row (too few columns)",
			csv:  "bad header row",
		},
		{
			name: "bad header (invalid column name)",
			csv: func() string {
				b := []byte(header)
				i := 10
				for b[i] == ',' || b[i] == 'X' {
					i++
				}
				b[i] = 'X'
				return string(b)
			}(),
		},
		{
			name: "bad book (header is invalid book)",
			csv:  header + "\n" + header,
		},
		{
			name:   "happy path",
			csv:    exampleCSV.csv,
			wantOk: true,
			want:   &Database{Books: happyPathSortedBooks},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := strings.NewReader(test.csv)
			got, err := NewDatabase(r)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("not equal: \n wanted: %v \n got:    %v", test.want, got) // comment out if running backfill
			}
		})
	}
}

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
		{"zero offset", 2, 0, []book.Subject{{Name: "animals", Count: 3}, {Name: "liquids", Count: 1}}},
		{"middle", 1, 1, []book.Subject{{Name: "liquids", Count: 1}}},
		{"Last only", 3, 2, []book.Subject{{Name: "plants", Count: 2}}},
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

func TestReadBook(t *testing.T) {
	tests := []struct {
		name   string
		books  []book.Book
		id     string
		wantOk bool
		want   *book.Book
	}{
		{
			name: "no books",
		},
		{
			name: "no book with id",
			id:   "abc",
			books: []book.Book{
				{Header: book.Header{ID: "def"}},
			},
		},
		{
			name: "happy path",
			id:   "def",
			books: []book.Book{
				{Header: book.Header{ID: "abc"}},
				{Header: book.Header{ID: "def"}},
				{Header: book.Header{ID: "ghi"}},
			},
			wantOk: true,
			want:   &book.Book{Header: book.Header{ID: "def"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := Database{
				Books: test.books,
			}
			got, err := d.ReadBook(test.id)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("not equal: \n wanted: %v \n got:    %v", test.want, got)
			}
		})
	}
}

func TestNotAllowed(t *testing.T) {
	tests := []struct {
		name string
		f    func(d *Database) error
	}{
		{"CreateBooks", func(d *Database) error { _, err := d.CreateBooks(); return err }},
		{"UpdateBook", func(d *Database) error { return d.UpdateBook(book.Book{}, false) }},
		{"DeleteBook", func(d *Database) error { return d.DeleteBook("id") }},
		{"ReadAdminPassword", func(d *Database) error { _, err := d.ReadAdminPassword(); return err }},
		{"UpdateAdminPassword", func(d *Database) error { return d.UpdateAdminPassword("Bilbo123") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := new(Database)
			if err := test.f(db); err == nil {
				t.Errorf("wanted error")
			}
		})
	}
}

func TestBookRecord(t *testing.T) {
	r := []string{
		"1",
		"2",
		"3",
		"5",
		"4", // subject should appear after description in csv
		"6",
		"7",
		"8",
		"07/04/2001",
		"11/16/2022",
		"11",
		"12",
		"13",
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
		PublishDate:   time.Date(2001, 7, 4, 0, 0, 0, 0, time.UTC),
		AddedDate:     time.Date(2022, 11, 16, 0, 0, 0, 0, time.UTC),
		EAN_ISBN13:    "11",
		UPC_ISBN10:    "12",
		ImageBase64:   "13",
	}
	t.Run("bookFromRecord", func(t *testing.T) {
		t.Run("too short", func(t *testing.T) {
			if _, err := bookFromRecord([]string{"single"}); err == nil {
				t.Errorf("wanted error for record with one column")
			}
		})
		want := &b
		got, err := bookFromRecord(r)
		switch {
		case err != nil:
			t.Errorf("unwanted error: %v", err)
		case !reflect.DeepEqual(want, got):
			t.Errorf("not equal: \n wanted: %v \n got:    %v", want, got)
		}
	})
	t.Run("record (to book)", func(t *testing.T) {
		if want, got := r, record(b); !reflect.DeepEqual(want, got) {
			t.Errorf("not equal: \n wanted: %v \n got:    %v", want, got)
		}
	})
}

func TestDump(t *testing.T) {
	books := exampleCSV.books
	want := exampleCSV.csv
	var sb strings.Builder
	d := NewDump(&sb)
	d.Write(books[0])
	d.Write(books[1:]...)
	got := sb.String()
	if want != got {
		t.Errorf("not equal: \n wanted: %v \n got:    %v", want, got)
	}
}

var exampleCSV = struct {
	csv   string
	books []book.Book
}{
	csv: header + `
1,2,3,5,4,6,7,8,07/04/2001,11/16/2022,11,12,13
id1,title2,author3,description5,subject4,ddc6,32,publisher8,01/11/2008,06/01/2020,ean11,upc12,image13
xyz*34,Thoughts,Anonymous,"Many essays about ""life,"" abridged.",poems,88.79,123,the world,07/04/2009,08/26/2022,xxx,yyy,zzz
`,
	books: []book.Book{
		{
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
			PublishDate:   time.Date(2001, 7, 4, 0, 0, 0, 0, time.UTC),
			AddedDate:     time.Date(2022, 11, 16, 0, 0, 0, 0, time.UTC),
			EAN_ISBN13:    "11",
			UPC_ISBN10:    "12",
			ImageBase64:   "13",
		},
		{
			Header: book.Header{
				ID:      "id1",
				Title:   "title2",
				Author:  "author3",
				Subject: "subject4",
			},
			Description:   "description5",
			DeweyDecClass: "ddc6",
			Pages:         32,
			Publisher:     "publisher8",
			PublishDate:   time.Date(2008, 1, 11, 0, 0, 0, 0, time.UTC),
			AddedDate:     time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC),
			EAN_ISBN13:    "ean11",
			UPC_ISBN10:    "upc12",
			ImageBase64:   "image13",
		},
		{
			Header: book.Header{
				ID:      "xyz*34",
				Title:   "Thoughts",
				Author:  "Anonymous",
				Subject: "poems",
			},
			Description:   `Many essays about "life," abridged.`,
			DeweyDecClass: "88.79",
			Pages:         123,
			Publisher:     "the world",
			PublishDate:   time.Date(2009, 7, 4, 0, 0, 0, 0, time.UTC),
			AddedDate:     time.Date(2022, 8, 26, 0, 0, 0, 0, time.UTC),
			EAN_ISBN13:    "xxx",
			UPC_ISBN10:    "yyy",
			ImageBase64:   "zzz",
		},
	},
}

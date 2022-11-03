// Package csv provides a read-only database for the library from the embedded CSV file.
package csv

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
)

//go:embed library.csv
var libraryCSV []byte

type Database struct {
	Books []book.Book
}

func NewDatabase() (*Database, error) {
	r := csv.NewReader(bytes.NewReader(libraryCSV))
	records, err := r.ReadAll()
	if err != nil {
		log.Fatalf("reading library csv: %v", err)
	}
	wantHeader := "id,title,author,description,subject,dewey-dec-class,pages,publisher,publish-date,added-date,ean-isbn13,upc-isbn10,image-base64\n"
	if len(records) == 0 || len(libraryCSV) < len(wantHeader) || string(libraryCSV[:len(wantHeader)]) != wantHeader {
		return nil, fmt.Errorf("invalid header row")
	}
	records = records[1:] // skip header row
	d := Database{
		Books: make([]book.Book, len(records)),
	}
	for i, r := range records {
		b, err := bookFromRecord(r)
		if err != nil {
			return nil, fmt.Errorf("reading book %v: %v", i, err)
		}
		d.Books[i] = *b
	}
	return &d, nil
}

func (d *Database) CreateBooks(books ...book.Book) ([]book.Book, error) {
	return nil, d.notAllowed()
}

func (d *Database) ReadBookHeaders(limit, offset int) ([]book.Header, error) {
	books := d.Books
	switch {
	case offset >= len(books):
		offset = len(books)
	case offset < 0:
		offset = 0
	}
	books = books[offset:]
	switch {
	case limit >= len(books):
		limit = len(books)
	case limit < 0:
		limit = 0
	}
	books = books[:limit]
	headers := make([]book.Header, len(books))
	for i, b := range books {
		headers[i] = b.Header
	}
	return headers, nil
}

func (d *Database) ReadBook(id string) (*book.Book, error) {
	for _, b := range d.Books {
		if b.ID == id {
			return &b, nil
		}
	}
	return nil, fmt.Errorf("no book with id of %q", id)
}

func (d *Database) UpdateBook(b book.Book, updateImage bool) error {
	return d.notAllowed()
}

func (d *Database) DeleteBook(id string) error {
	return d.notAllowed()
}

func (d *Database) ReadAdminPassword() (hashedPassword []byte, err error) {
	return nil, d.notAllowed()
}

func (d *Database) UpdateAdminPassword(hashedPassword string) error {
	return d.notAllowed()
}

func (d Database) notAllowed() error {
	return fmt.Errorf("not supported by %T", d)
}

func bookFromRecord(r []string) (*book.Book, error) {
	if n := len(r); n != 13 {
		return nil, fmt.Errorf("expected 13 rows, got %v", n)
	}
	var b book.Book
	fields := []struct {
		p   interface{}
		key string
	}{
		{&b.ID, "id"},
		{&b.Title, "title"},
		{&b.Author, "author"},
		{&b.Description, "description"},
		{&b.Subject, "subject"},
		{&b.DeweyDecClass, "dewey-dec-class"},
		{&b.Pages, "pages"},
		{&b.Publisher, "publisher"},
		{&b.PublishDate, "publish-date"},
		{&b.AddedDate, "added-date"},
		{&b.EAN_ISBN13, "ean-isbn-13"},
		{&b.UPC_ISBN10, "upc-isbn-10"},
		{&b.ImageBase64, "image-base64"},
	}
	for i, f := range fields {
		if err := parseFormValue(f.p, f.key, i, r); err != nil {
			return nil, err
		}
	}
	return &b, nil
}

func parseFormValue(p interface{}, key string, i int, r []string) error {
	v := r[i]
	if len(v) == 0 {
		return nil
	}
	var err error
	switch ptr := p.(type) {
	case *string:
		if len(v) == 0 {
			err = fmt.Errorf("value not set")
			break
		}
		*ptr = v
	case *int:
		var i int
		i, err = strconv.Atoi(v)
		if err != nil {
			break
		}
		*ptr = i
	case *time.Time:
		var t time.Time
		const DateLayout = "01/02/2006"
		t, err = time.Parse(DateLayout, v)
		if err != nil {
			break
		}
		*ptr = t
	}
	if err != nil {
		return fmt.Errorf("parsing key %q (column %v) (%q) as %T: %v", key, i, v, p, err)
	}
	return nil
}

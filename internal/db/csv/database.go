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
	Books       []book.Book
	BookHeaders []book.Header
}

func NewDatabase() (*Database, error) {
	r := csv.NewReader(bytes.NewReader(libraryCSV))
	records, err := r.ReadAll()
	if err != nil {
		log.Fatalf("reading library csv: %v", err)
	}
	db := Database{
		Books:       make([]book.Book, len(records)),
		BookHeaders: make([]book.Header, len(records)),
	}
	for i, r := range records[1:] { // skip header row
		b, err := bookFromRecord(r)
		if err != nil {
			return nil, fmt.Errorf("reading book %v: %v", i, err)
		}
		db.Books[i] = *b
		db.BookHeaders[i] = b.Header
	}
	return &db, nil
}

func (db *Database) CreateBooks(books ...book.Book) ([]book.Book, error) {
	return nil, db.notAllowed()
}

func (db *Database) ReadBooks() ([]book.Header, error) {
	return db.BookHeaders, nil
}

func (db *Database) ReadBook(id string) (*book.Book, error) {
	for _, b := range db.Books {
		if b.ID == id {
			return &b, nil
		}
	}
	return nil, fmt.Errorf("no book with id of %q", id)
}

func (db *Database) UpdateBook(b book.Book, updateImage bool) error {
	return db.notAllowed()
}

func (db *Database) DeleteBook(id string) error {
	return db.notAllowed()
}

func (db *Database) ReadAdminPassword() (hashedPassword []byte, err error) {
	return nil, db.notAllowed()
}

func (db *Database) UpdateAdminPassword(hashedPassword string) error {
	return db.notAllowed()
}

func (db Database) notAllowed() error {
	return fmt.Errorf("not supported by %T", db)
}

func bookFromRecord(r []string) (*book.Book, error) {
	// id,title,author,description,subject,dewey-dec-class,pages,publisher,publish-date,added-date,ean-isbn13,upc-isbn10,image-base64
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

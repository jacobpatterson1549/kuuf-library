// Package csv provides a read-only database for the library from the embedded CSV file.
package csv

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
)

//go:embed library.csv
var libraryCSV []byte

type Database struct {
	Books []book.Book
}

const header = "id,title,author,description,subject,dewey-dec-class,pages,publisher,publish-date,added-date,ean-isbn13,upc-isbn10,image-base64"
const dateLayout = book.SlashMMDDYYYY

var headerRecord = strings.Split(header, ",")

func NewDatabase() (*Database, error) {
	r := csv.NewReader(bytes.NewReader(libraryCSV))
	records, err := r.ReadAll()
	if err != nil {
		log.Fatalf("reading library csv: %v", err)
	}
	wantHeader := header + "\n"
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
			return nil, fmt.Errorf("reading book %v: %w", i, err)
		}
		d.Books[i] = *b
	}
	book.Books(d.Books).Sort()
	return &d, nil
}

func (d *Database) CreateBooks(books ...book.Book) ([]book.Book, error) {
	return nil, d.notAllowed()
}

func (d *Database) ReadBookSubjects(limit, offset int) ([]book.Subject, error) {
	if limit < 0 {
		return []book.Subject{}, nil
	}
	if offset < 0 {
		offset = 0
	}
	m := make(map[string]int)
	for _, b := range d.Books {
		m[b.Subject]++
	}
	if offset > len(m) {
		return []book.Subject{}, nil
	}
	subjects := make(book.Subjects, 0, len(m))
	for name, count := range m {
		s := book.Subject{
			Name:  name,
			Count: count,
		}
		subjects = append(subjects, s)
	}
	subjects.Sort()
	subjects = subjects[offset:]
	if len(subjects) > limit {
		subjects = subjects[:limit]
	}
	return subjects, nil
}

func (d *Database) ReadBookHeaders(filter book.Filter, limit, offset int) ([]book.Header, error) {
	books := d.Books
	if limit < 0 || offset > len(books) {
		return []book.Header{}, nil
	}
	if offset < 0 {
		offset = 0
	}
	headers := make([]book.Header, 0, limit+offset)
	for _, b := range books {
		if filter.Matches(b) {
			headers = append(headers, b.Header)
			if len(headers) == cap(headers) {
				break
			}
		}
	}
	headers = headers[offset:]
	if len(headers) > limit {
		headers = headers[:limit]
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
	if want, got := len(headerRecord), len(r); want != got {
		return nil, fmt.Errorf("expected %v columns, got %v", want, got)
	}
	var sb book.StringBook
	sb.ID = r[0]
	sb.Title = r[1]
	sb.Author = r[2]
	sb.Description = r[3]
	sb.Subject = r[4]
	sb.DeweyDecClass = r[5]
	sb.Pages = r[6]
	sb.Publisher = r[7]
	sb.PublishDate = r[8]
	sb.AddedDate = r[9]
	sb.EAN_ISBN13 = r[10]
	sb.UPC_ISBN10 = r[11]
	sb.ImageBase64 = r[12]
	return sb.Book(dateLayout)
}

func record(b book.Book) []string {
	return []string{
		b.ID,
		b.Title,
		b.Author,
		b.Description,
		b.Subject,
		b.DeweyDecClass,
		strconv.Itoa(b.Pages),
		b.Publisher,
		b.PublishDate.Format(string(dateLayout)),
		b.AddedDate.Format(string(dateLayout)),
		b.EAN_ISBN13,
		b.UPC_ISBN10,
		b.ImageBase64,
	}
}

type Dump struct {
	w *csv.Writer
}

func NewDump(w io.Writer) *Dump {
	d := Dump{
		w: csv.NewWriter(w),
	}
	d.w.Write(headerRecord)
	return &d
}

func (d *Dump) Write(books ...book.Book) {
	for _, b := range books {
		d.w.Write(record(b))
	}
	d.w.Flush()
}

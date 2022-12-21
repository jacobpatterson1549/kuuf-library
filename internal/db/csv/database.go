// Package csv provides a read-only database for the library from the embedded CSV file.
package csv

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
)

type Database struct {
	Books []book.Book
}

const (
	header     = "id,title,author,description,subject,dewey-dec-class,pages,publisher,publish-date,added-date,ean-isbn13,upc-isbn10,image-base64"
	dateLayout = book.SlashMMDDYYYY
)

var headerRecord = strings.Split(header, ",")

func NewDatabase(r io.Reader) (*Database, error) {
	records, err := readRecords(r)
	if err != nil {
		return nil, err
	}
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

func readRecords(r io.Reader) ([][]string, error) {
	csvR := csv.NewReader(r)
	records, err := csvR.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading library csv: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	wantHeader := headerRecord
	gotHeader := records[0]
	if len(wantHeader) != len(gotHeader) {
		return nil, fmt.Errorf("header too short/long: wanted %q", header)
	}
	for i := range wantHeader {
		if want, got := wantHeader[i], gotHeader[i]; want != got {
			return nil, fmt.Errorf("header column %v: wanted %q, got %q", i, want, got)
		}
	}
	records = records[1:] // skip header row
	return records, nil
}

func (d Database) ReadBookSubjects(limit, offset int) ([]book.Subject, error) {
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

func (d Database) ReadBookHeaders(filter book.Filter, limit, offset int) ([]book.Header, error) {
	books := d.Books
	if limit < 0 || offset > len(books) {
		return []book.Header{}, nil
	}
	if offset < 0 {
		offset = 0
	}
	headers := make([]book.Header, 0, limit+offset)
	for _, b := range books {
		if !filter.Matches(b) {
			continue
		}
		headers = append(headers, b.Header)
		if len(headers) == cap(headers) {
			break
		}
	}
	headers = headers[offset:]
	if len(headers) > limit {
		headers = headers[:limit]
	}
	return headers, nil
}

func (d Database) ReadBook(id string) (*book.Book, error) {
	for _, b := range d.Books {
		if b.ID == id {
			return &b, nil
		}
	}
	return nil, fmt.Errorf("no book with id of %q", id)
}

func bookFromRecord(r []string) (*book.Book, error) {
	if want, got := len(headerRecord), len(r); want != got {
		return nil, fmt.Errorf("expected %v columns, got %v", want, got)
	}
	sb := book.StringBook{
		ID:            r[0],
		Title:         r[1],
		Author:        r[2],
		Description:   r[3],
		Subject:       r[4],
		DeweyDecClass: r[5],
		Pages:         r[6],
		Publisher:     r[7],
		PublishDate:   r[8],
		AddedDate:     r[9],
		EanIsbn13:     r[10],
		UpcIsbn10:     r[11],
		ImageBase64:   r[12],
	}
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
		b.EanIsbn13,
		b.UpcIsbn10,
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

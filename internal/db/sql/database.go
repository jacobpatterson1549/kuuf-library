// Package sql provides a database for the library.
package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	_ "github.com/lib/pq"           // register "postgres" database driver from package init() function
	_ "github.com/mattn/go-sqlite3" // register "sqlite3" database driver from package init() function
)

type (
	Database struct {
		*db
		driver driverInfo
	}
	driverInfo struct {
		ILike string
	}
	query struct {
		cmd                string
		args               []interface{}
		wantedRowsAffected []int64
	}
)

var drivers = map[string]driverInfo{
	"postgres": {"ILIKE"},
	"sqlite3":  {"LIKE"},
}

func NewDatabase(ctx context.Context, driverName, url string) (*Database, error) {
	driver, ok := drivers[driverName]
	if !ok {
		return nil, fmt.Errorf("unknown driverName: %q", driverName)
	}
	sqlDB, err := sql.Open(driverName, url)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	d := Database{
		db:     &db{sqlDB},
		driver: driver,
	}
	if err := d.setupTables(ctx); err != nil {
		return nil, fmt.Errorf("setting up tables: %w", err)
	}
	return &d, nil
}

func (d *Database) setupTables(ctx context.Context) error {
	queries := []query{
		{
			cmd: "CREATE TABLE IF NOT EXISTS books" +
				" ( id TEXT PRIMARY KEY" +
				" , title TEXT" +
				" , author TEXT" +
				" , subject TEXT" +
				" , description TEXT" +
				" , dewey_dec_class TEXT" +
				" , pages INT" +
				" , publisher TEXT" +
				" , publish_date TIMESTAMP" +
				" , added_date TIMESTAMP" +
				" , ean_isbn13 TEXT" +
				" , upc_isbn10 TEXT" +
				" , image_base64 TEXT" +
				" )",
			wantedRowsAffected: []int64{0},
		},
		{
			cmd: "CREATE TABLE IF NOT EXISTS users" +
				" ( username TEXT PRIMARY KEY" +
				" , password TEXT" +
				" )",
			wantedRowsAffected: []int64{0},
		},
		{
			cmd: "INSERT INTO users (username)" +
				" VALUES ('admin')" +
				" ON CONFLICT DO NOTHING",
			wantedRowsAffected: []int64{0, 1},
		},
	}
	return d.execTx(ctx, queries...)
}

func (d *Database) CreateBooks(ctx context.Context, books ...book.Book) ([]book.Book, error) {
	queries := make([]query, len(books))
	created := make([]book.Book, len(books))
	for i, b := range books {
		b.ID = book.NewID()
		queries[i].cmd = "INSERT INTO books (id, title, author, subject, description, dewey_dec_class, pages, publisher, publish_date, added_date, ean_isbn13, upc_isbn10, image_base64)" +
			" VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)"
		queries[i].args = []interface{}{b.ID, b.Title, b.Author, b.Subject, b.Description, b.DeweyDecClass, b.Pages, b.Publisher, b.PublishDate, b.AddedDate, b.EanIsbn13, b.UpcIsbn10, b.ImageBase64}
		queries[i].wantedRowsAffected = []int64{1}
		created[i] = b
	}
	if err := d.execTx(ctx, queries...); err != nil {
		return nil, fmt.Errorf("creating books: %w", err)
	}
	return created, nil
}

func (d *Database) ReadBookSubjects(ctx context.Context, limit, offset int) ([]book.Subject, error) {
	cmd := "SELECT subject, COUNT(*)" +
		" FROM books" +
		" GROUP BY subject" +
		" ORDER BY subject ASC" +
		" LIMIT $1" +
		" OFFSET $2"
	q := query{
		cmd:  cmd,
		args: []interface{}{limit, offset},
	}
	subjects := make([]book.Subject, limit)
	n := 0
	dest := func() []interface{} {
		if n >= limit {
			return nil
		}
		s := &subjects[n]
		n++
		return []interface{}{&s.Name, &s.Count}
	}
	if err := d.query(ctx, q, dest); err != nil {
		return nil, fmt.Errorf("reading book subjects: %w", err)
	}
	subjects = subjects[:n]
	return subjects, nil
}

func (d *Database) ReadBookHeaders(ctx context.Context, filter book.Filter, limit, offset int) ([]book.Header, error) {
	hasSubject := len(filter.Subject) != 0
	hasHeaderPart := len(filter.HeaderPart) != 0
	likeHeaderPart := "%" + filter.HeaderPart + "%"
	cmd := "SELECT id, title, author, subject" +
		" FROM books" +
		" WHERE ($1 OR subject = $2)" +
		" AND ($3" +
		" OR title " + d.driver.ILike + " $4" +
		" OR author " + d.driver.ILike + " $4" +
		" OR subject " + d.driver.ILike + " $4)" +
		" ORDER BY subject ASC, Title ASC" +
		" LIMIT $5" +
		" OFFSET $6"
	q := query{
		cmd:  cmd,
		args: []interface{}{!hasSubject, filter.Subject, !hasHeaderPart, likeHeaderPart, limit, offset},
	}
	headers := make([]book.Header, limit)
	n := 0
	dest := func() []interface{} {
		if n >= limit {
			return nil
		}
		h := &headers[n]
		n++
		return []interface{}{&h.ID, &h.Title, &h.Author, &h.Subject}
	}
	if err := d.query(ctx, q, dest); err != nil {
		return nil, fmt.Errorf("reading book headers: %w", err)
	}
	headers = headers[:n]
	return headers, nil
}

func (d *Database) ReadBook(ctx context.Context, id string) (*book.Book, error) {
	cmd := "SELECT id, title, author, subject, description, dewey_dec_class, pages, publisher, publish_date, added_date, ean_isbn13, upc_isbn10, image_base64" +
		" FROM books" +
		" WHERE id = $1"
	q := query{
		cmd:  cmd,
		args: []interface{}{id},
	}
	var b book.Book
	dest := []interface{}{&b.ID, &b.Title, &b.Author, &b.Subject, &b.Description, &b.DeweyDecClass, &b.Pages, &b.Publisher, &b.PublishDate, &b.AddedDate, &b.EanIsbn13, &b.UpcIsbn10, &b.ImageBase64}
	if err := d.queryRow(ctx, q, dest...); err != nil {
		return nil, fmt.Errorf("reading book: %w", err)
	}
	return &b, nil
}

func (d *Database) UpdateBook(ctx context.Context, b book.Book, updateImage bool) error {
	cmd := "UPDATE books" +
		" SET title = $1, author = $2, subject = $3, description = $4, dewey_dec_class = $5, pages = $6, publisher = $7, publish_date = $8, added_date = $9, ean_isbn13 = $10, upc_isbn10 = $11"
	args := []interface{}{b.Title, b.Author, b.Subject, b.Description, b.DeweyDecClass, b.Pages, b.Publisher, b.PublishDate, b.AddedDate, b.EanIsbn13, b.UpcIsbn10}
	if updateImage {
		cmd += ", image_base64 = $12 WHERE id = $13"
		args = append(args, b.ImageBase64, b.ID)
	} else {
		cmd += " WHERE id = $12"
		args = append(args, b.ID)
	}
	q := query{
		cmd:                cmd,
		args:               args,
		wantedRowsAffected: []int64{1},
	}
	if err := d.execTx(ctx, q); err != nil {
		return fmt.Errorf("updating book: %w", err)
	}
	return nil
}

func (d *Database) DeleteBook(ctx context.Context, id string) error {
	cmd := "DELETE FROM books WHERE id = $1"
	q := query{
		cmd:                cmd,
		args:               []interface{}{id},
		wantedRowsAffected: []int64{1},
	}
	if err := d.execTx(ctx, q); err != nil {
		return fmt.Errorf("deleting book: %w", err)
	}
	return nil
}

func (d *Database) ReadAdminPassword(ctx context.Context) (hashedPassword []byte, err error) {
	cmd := "SELECT password FROM users WHERE username = $1"
	q := query{
		cmd:  cmd,
		args: []interface{}{"admin"},
	}
	if err := d.queryRow(ctx, q, &hashedPassword); err != nil {
		return nil, fmt.Errorf("reading admin password: %w", err)
	}
	return hashedPassword, nil
}

func (d *Database) UpdateAdminPassword(ctx context.Context, hashedPassword string) error {
	cmd := "UPDATE users SET password = $1 WHERE username = $2"
	q := query{
		cmd:                cmd,
		args:               []interface{}{hashedPassword, "admin"},
		wantedRowsAffected: []int64{1},
	}
	if err := d.execTx(ctx, q); err != nil {
		return fmt.Errorf("updating admin password: %w", err)
	}
	return nil
}

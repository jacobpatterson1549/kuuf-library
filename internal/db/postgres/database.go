// Package postgres provides a database for the library.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	_ "github.com/lib/pq" // register "postgres" database driver from package init() function
)

type Database struct {
	db           *sql.DB
	QueryTimeout time.Duration
}

const DriverName = "postgres"

func NewDatabase(url string, queryTimeout time.Duration) (*Database, error) {
	db, err := sql.Open(DriverName, url)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	d := Database{
		db:           db,
		QueryTimeout: queryTimeout,
	}
	if err := d.setupTables(); err != nil {
		return nil, fmt.Errorf("setting up tables: %w", err)
	}
	return &d, nil
}

func (d *Database) setupTables() error {
	cmds := []string{
		`CREATE TABLE IF NOT EXISTS books
		( _id SERIAL PRIMARY KEY
		, id CHAR(32) UNIQUE
		, title VARCHAR
		, author VARCHAR
		, subject TEXT
		, description TEXT
		, dewey_dec_class VARCHAR
		, pages INT
		, publisher VARCHAR
		, publish_date TIMESTAMP
		, added_date TIMESTAMP
		, ean_isbn13 VARCHAR
		, upc_isbn10 VARCHAR
		, image_base64 TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS users
		( username VARCHAR(32) PRIMARY KEY
		, password CHAR(60)
		)`,
		`INSERT INTO users (username)
		VALUES ('admin')
		ON CONFLICT DO NOTHING
		`,
	}
	queries := make([]query, len(cmds))
	for i, cmd := range cmds {
		queries[i].cmd = cmd
	}
	return d.execTx(queries...)
}

type query struct {
	cmd  string
	args []interface{}
}

func (d *Database) withTimeoutContext(f func(context.Context) error) error {
	ctx := context.Background()
	ctx, cancelFunc := context.WithTimeout(ctx, d.QueryTimeout)
	defer cancelFunc()
	return f(ctx)
}

func (d *Database) execTx(queries ...query) error {
	return d.withTimeoutContext(func(ctx context.Context) error {
		tx, err := d.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning transaction: %w", err)
		}
		for _, q := range queries {
			if _, err = tx.Exec(q.cmd, q.args...); err != nil {
				break
			}
		}
		if err != nil {
			if err2 := tx.Rollback(); err2 != nil {
				err = fmt.Errorf("rollback error: %v, root cause: %w", err, err2)
			}
			return fmt.Errorf("executing transaction queries: %w", err)
		}
		if err != tx.Commit() {
			return fmt.Errorf("committing transaction: %w", err)
		}
		return nil
	})
}

func (d *Database) query(q query, dest func() []interface{}) error {
	return d.withTimeoutContext(func(ctx context.Context) error {
		rows, err := d.db.QueryContext(ctx, q.cmd, q.args...)
		if err != nil {
			return fmt.Errorf("running query: %w", err)
		}
		defer rows.Close()
		var i int
		for rows.Next() {
			if err := rows.Scan(dest()...); err != nil {
				return fmt.Errorf("scanning row %v: %w", i, err)
			}
			i++
		}
		return nil
	})
}

func (d *Database) CreateBooks(books ...book.Book) ([]book.Book, error) {
	queries := make([]query, len(books))
	for i, b := range books {
		b.ID = book.NewID()
		queries[i].cmd = `INSERT INTO books (id, title, author, subject, description, dewey_dec_class, pages, publisher, publish_date, added_date, ean_isbn13, upc_isbn10, image_base64)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`
		queries[i].args = []interface{}{b.ID, b.Title, b.Author, b.Subject, b.Description, b.DeweyDecClass, b.Pages, b.Publisher, b.PublishDate, b.AddedDate, b.EAN_ISBN13, b.UPC_ISBN10, b.ImageBase64}
	}
	if err := d.execTx(queries...); err != nil {
		return nil, fmt.Errorf("creating books: %w", err)
	}
	return books, nil
}

func (d *Database) ReadBookHeaders(limit, offset int) ([]book.Header, error) {
	cmd := `SELECT id, title, author, subject
	FROM books
	LIMIT $1
	OFFSET $2`
	q := query{
		cmd:  cmd,
		args: []interface{}{limit, offset},
	}
	var refs []*book.Header
	dest := func() []interface{} {
		var h book.Header
		refs = append(refs, &h)
		return []interface{}{&h.ID, &h.Title, &h.Author, &h.Subject}
	}
	if err := d.query(q, dest); err != nil {
		return nil, fmt.Errorf("making query: %w", err)
	}
	headers := make([]book.Header, len(refs))
	for i, h := range refs {
		headers[i] = *h
	}
	return headers, nil
}

func (d *Database) ReadBook(id string) (*book.Book, error) {
	cmd := `SELECT id, title, author, subject, description, dewey_dec_class, pages, publisher, publish_date, added_date, ean_isbn13, upc_isbn10, image_base64
	FROM books
	WHERE id = $1`
	var b book.Book
	q := query{
		cmd:  cmd,
		args: []interface{}{id},
	}
	dest := func() []interface{} {
		return []interface{}{&b.ID, &b.Title, &b.Author, &b.Subject, &b.Description, &b.DeweyDecClass, &b.Pages, &b.Publisher, &b.PublishDate, &b.AddedDate, &b.EAN_ISBN13, &b.UPC_ISBN10, &b.ImageBase64}
	}
	if err := d.query(q, dest); err != nil {
		return nil, fmt.Errorf("making query: %w", err)
	}
	return &b, nil
}

func (d *Database) UpdateBook(b book.Book, newID string, updateImage bool) error {
	cmd := `UPDATE books SET id = $1, title = $2, author = $3, subject = $4, description = $5, dewey_dec_class = $6, pages = $7, publisher = $8, publish_date = $9, added_date = $10, ean_isbn13 = $11, upc_isbn10 = $12`
	args := []interface{}{newID, b.Title, b.Author, b.Subject, b.Description, b.DeweyDecClass, b.Pages, b.Publisher, b.PublishDate, b.AddedDate, b.EAN_ISBN13, b.UPC_ISBN10}
	if updateImage {
		cmd += `, image_base64 = $13 WHERE id = $14`
		args = append(args, b.ImageBase64, b.ID)
	} else {
		cmd += ` WHERE id = $13`
		args = append(args, b.ID)
	}
	q := query{
		cmd:  cmd,
		args: args,
	}
	if err := d.execTx(q); err != nil {
		return fmt.Errorf("updating book: %w", err)
	}
	return nil
}

func (d *Database) DeleteBook(id string) error {
	cmd := `DELETE FROM books WHERE id = $1`
	q := query{
		cmd:  cmd,
		args: []interface{}{id},
	}
	if err := d.execTx(q); err != nil {
		return fmt.Errorf("deleting book: %w", err)
	}
	return nil
}

func (d *Database) ReadAdminPassword() (hashedPassword []byte, err error) {
	cmd := `SELECT password FROM users WHERE username = $1`
	q := query{
		cmd:  cmd,
		args: []interface{}{"admin"},
	}
	dest := func() []interface{} {
		return []interface{}{&hashedPassword}
	}
	if err := d.query(q, dest); err != nil {
		return nil, fmt.Errorf("reading admin password: %w", err)
	}
	return hashedPassword, nil
}

func (d *Database) UpdateAdminPassword(hashedPassword string) error {
	cmd := `UPDATE users SET password = $1 where username = $2`
	q := query{
		cmd:  cmd,
		args: []interface{}{hashedPassword, "admin"},
	}
	if err := d.execTx(q); err != nil {
		return fmt.Errorf("updating admin password: %w", err)
	}
	return nil
}

package server

import (
	"context"
	"fmt"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
)

// bookIterator reads books in batches
type bookIterator struct {
	database     database
	batchSize    int
	batchIndex   int
	headerIndex  int
	batchHeaders []book.Header
	closed       bool
	nextErr      error
}

type AllBooksDatabase interface {
	AllBooks() ([]book.Book, error)
}

type allBooksDatabase struct {
	database
	AllBooksFunc func() ([]book.Book, error)
}

func (abi allBooksDatabase) AllBooks() ([]book.Book, error) {
	return abi.AllBooksFunc()
}

func newBookIterator(database database, batchSize int) *bookIterator {
	iter := bookIterator{
		database:  database,
		batchSize: batchSize,
	}
	return &iter
}

// HasNext moves the iterator, requesting book headers if needed.
func (iter *bookIterator) HasNext(ctx context.Context) bool {
	switch {
	case iter.closed,
		iter.batchIndex != 0 &&
			iter.batchSize >= len(iter.batchHeaders) &&
			iter.headerIndex >= len(iter.batchHeaders):
		iter.closed = true
		return false
	case iter.batchIndex == 0,
		iter.headerIndex >= len(iter.batchHeaders)-1 && iter.batchSize < len(iter.batchHeaders): // request more headers
		var filter book.Filter
		limit := iter.batchSize + 1
		offset := iter.batchSize * iter.batchIndex
		headers, err := iter.database.ReadBookHeaders(ctx, filter, limit, offset)
		if err != nil {
			iter.closed = true
			iter.nextErr = fmt.Errorf("requesting more headers: %w", err)
			return false
		}
		iter.batchHeaders = headers
		iter.batchIndex++
		iter.headerIndex = 0
		if len(headers) == 0 {
			iter.closed = true
			return false
		}
	}
	return true
}

// Next requests a book if the iterator has a next book
func (iter *bookIterator) Next(ctx context.Context) (*book.Book, error) {
	switch {
	case iter.nextErr != nil:
		err := iter.nextErr
		iter.nextErr = nil
		return nil, err
	case iter.closed:
		return nil, fmt.Errorf("iterator closed")
	}
	header := iter.batchHeaders[iter.headerIndex]
	iter.headerIndex++
	b, err := iter.database.ReadBook(ctx, header.ID)
	if err != nil {
		return nil, fmt.Errorf("reading book: %w", err)
	}
	return b, nil
}

func (iter *bookIterator) Err() error {
	return iter.nextErr
}

func (iter *bookIterator) AllBooks(ctx context.Context) ([]book.Book, error) {
	if abi, ok := iter.database.(AllBooksDatabase); ok {
		return abi.AllBooks()
	}
	var books []book.Book
	for iter.HasNext(ctx) {
		b, err := iter.Next(ctx)
		if err != nil {
			return nil, fmt.Errorf("reading book: %w", err)
		}
		books = append(books, *b)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return books, nil
}

// readOnlyDatabase is a database that only reads books.
type readOnlyDatabase struct {
	ReadBookSubjectsFunc func(ctx context.Context, limit, offset int) ([]book.Subject, error)
	ReadBookHeadersFunc  func(ctx context.Context, filter book.Filter, limit, offset int) ([]book.Header, error)
	ReadBookFunc         func(ctx context.Context, id string) (*book.Book, error)
}

var _ database = readOnlyDatabase{}

func (d readOnlyDatabase) CreateBooks(ctx context.Context, books ...book.Book) ([]book.Book, error) {
	return nil, d.notAllowed()
}

func (d readOnlyDatabase) ReadBookSubjects(ctx context.Context, limit, offset int) ([]book.Subject, error) {
	return d.ReadBookSubjectsFunc(ctx, limit, offset)
}

func (d readOnlyDatabase) ReadBookHeaders(ctx context.Context, filter book.Filter, limit, offset int) ([]book.Header, error) {
	return d.ReadBookHeadersFunc(ctx, filter, limit, offset)
}

func (d readOnlyDatabase) ReadBook(ctx context.Context, id string) (*book.Book, error) {
	return d.ReadBookFunc(ctx, id)
}

func (d readOnlyDatabase) UpdateBook(ctx context.Context, b book.Book, updateImage bool) error {
	return d.notAllowed()
}

func (d readOnlyDatabase) DeleteBook(ctx context.Context, id string) error {
	return d.notAllowed()
}

func (d readOnlyDatabase) ReadAdminPassword(ctx context.Context) (hashedPassword []byte, err error) {
	return nil, d.notAllowed()
}

func (d readOnlyDatabase) UpdateAdminPassword(ctx context.Context, hashedPassword string) error {
	return d.notAllowed()
}

func (d readOnlyDatabase) notAllowed() error {
	return fmt.Errorf("not supported")
}

package server

import (
	"github.com/jacobpatterson1549/kuuf-library/internal/book"
)

type countRateLimiter struct {
	count, max int
}

func (m *countRateLimiter) Allow() bool {
	m.count++
	return m.count <= m.max
}

type mockPasswordHandler struct {
	hashFunc              func(password []byte) (hashedPassword []byte, err error)
	isCorrectPasswordFunc func(hashedPassword, password []byte) (ok bool, err error)
}

func (m mockPasswordHandler) Hash(password []byte) (hashedPassword []byte, err error) {
	return m.hashFunc(password)
}

func (m mockPasswordHandler) IsCorrectPassword(hashedPassword, password []byte) (ok bool, err error) {
	return m.isCorrectPasswordFunc(hashedPassword, password)
}

type mockDatabase struct {
	createBooksFunc         func(books ...book.Book) ([]book.Book, error)
	readBookSubjectsFunc    func(limit, offset int) ([]book.Subject, error)
	readBookHeadersFunc     func(f book.Filter, limit, offset int) ([]book.Header, error)
	readBookFunc            func(id string) (*book.Book, error)
	updateBookFunc          func(b book.Book, updateImage bool) error
	deleteBookFunc          func(id string) error
	readAdminPasswordFunc   func() (hashedPassword []byte, err error)
	updateAdminPasswordFunc func(hashedPassword string) error
}

func (m mockDatabase) CreateBooks(books ...book.Book) ([]book.Book, error) {
	return m.createBooksFunc(books...)
}

func (m mockDatabase) ReadBookSubjects(limit, offset int) ([]book.Subject, error) {
	return m.readBookSubjectsFunc(limit, offset)
}

func (m mockDatabase) ReadBookHeaders(f book.Filter, limit, offset int) ([]book.Header, error) {
	return m.readBookHeadersFunc(f, limit, offset)
}

func (m mockDatabase) ReadBook(id string) (*book.Book, error) {
	return m.readBookFunc(id)
}

func (m mockDatabase) UpdateBook(b book.Book, updateImage bool) error {
	return m.updateBookFunc(b, updateImage)
}

func (m mockDatabase) DeleteBook(id string) error {
	return m.deleteBookFunc(id)
}

func (m mockDatabase) ReadAdminPassword() (hashedPassword []byte, err error) {
	return m.readAdminPasswordFunc()
}

func (m mockDatabase) UpdateAdminPassword(hashedPassword string) error {
	return m.updateAdminPasswordFunc(hashedPassword)
}

package server

import "github.com/jacobpatterson1549/kuuf-library/internal/book"

type mockPasswordHandler struct {
	mockHashFunc              func(password []byte) (hashedPassword []byte, err error)
	mockIsCorrectPasswordFunc func(hashedPassword, password []byte) (ok bool, err error)
}

func (m mockPasswordHandler) Hash(password []byte) (hashedPassword []byte, err error) {
	return m.mockHashFunc(password)
}

func (m mockPasswordHandler) IsCorrectPassword(hashedPassword, password []byte) (ok bool, err error) {
	return m.mockIsCorrectPasswordFunc(hashedPassword, password)
}

type mockDatabase struct {
	mockCreateBooksFunc         func(books ...book.Book) ([]book.Book, error)
	mockReadBookHeadersFunc     func(f book.Filter, limit, offset int) ([]book.Header, error)
	mockReadBookFunc            func(id string) (*book.Book, error)
	mockUpdateBookFunc          func(b book.Book, newID string, updateImage bool) error
	mockDeleteBookFunc          func(id string) error
	mockReadAdminPasswordFunc   func() (hashedPassword []byte, err error)
	mockUpdateAdminPasswordFunc func(hashedPassword string) error
}

func (m mockDatabase) CreateBooks(books ...book.Book) ([]book.Book, error) {
	return m.mockCreateBooksFunc(books...)
}

func (m mockDatabase) ReadBookHeaders(f book.Filter, limit, offset int) ([]book.Header, error) {
	return m.mockReadBookHeadersFunc(f, limit, offset)
}

func (m mockDatabase) ReadBook(id string) (*book.Book, error) {
	return m.mockReadBookFunc(id)
}

func (m mockDatabase) UpdateBook(b book.Book, newID string, updateImage bool) error {
	return m.mockUpdateBookFunc(b, newID, updateImage)
}

func (m mockDatabase) DeleteBook(id string) error {
	return m.mockDeleteBookFunc(id)
}

func (m mockDatabase) ReadAdminPassword() (hashedPassword []byte, err error) {
	return m.mockReadAdminPasswordFunc()
}

func (m mockDatabase) UpdateAdminPassword(hashedPassword string) error {
	return m.mockUpdateAdminPasswordFunc(hashedPassword)
}

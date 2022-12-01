package server

import (
	"fmt"
	"html"
	"net/http"
	"strconv"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
)

func (s *Server) getBookSubjects(w http.ResponseWriter, r *http.Request) {
	if data, ok := loadPage(w, r, s.cfg.MaxRows, "Subjects", s.db.ReadBookSubjects); ok {
		s.serveTemplate(w, "subjects", data)
	}
}

func (s *Server) getBookHeaders(w http.ResponseWriter, r *http.Request) {
	var headerParts string
	if !ParseFormValue(w, r, "q", &headerParts, 256) {
		return
	}
	var subject string
	if !ParseFormValue(w, r, "s", &subject, 256) {
		return
	}
	filter := book.NewFilter(headerParts, subject)
	pageLoader := func(limit, offset int) ([]book.Header, error) {
		return s.db.ReadBookHeaders(*filter, limit, offset)
	}
	if data, ok := loadPage(w, r, s.cfg.MaxRows, "Books", pageLoader); ok {
		data["Filter"] = headerParts
		data["Subject"] = subject
		s.serveTemplate(w, "list", data)
	}
}

func (s *Server) getBook(w http.ResponseWriter, r *http.Request) {
	var id string
	if !ParseFormValue(w, r, "id", &id, 64) {
		return
	}
	b, err := s.db.ReadBook(id)
	if err != nil {
		httpInternalServerError(w, err)
		return
	}
	s.serveTemplate(w, "book", b)
}

func (s *Server) getAdmin(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Book               book.Book
		ValidPasswordRunes string
	}{
		ValidPasswordRunes: html.EscapeString(validPasswordRunes),
	}
	hasID := r.URL.Query().Has("book-id")
	if hasID {
		id := r.URL.Query().Get("book-id")
		b, err := s.db.ReadBook(id)
		if err != nil {
			httpInternalServerError(w, err)
			return
		}
		data.Book = *b
	}
	s.serveTemplate(w, "admin", data)
}

func (s *Server) postBook(w http.ResponseWriter, r *http.Request) {
	b, err := bookFrom(w, r)
	if err != nil {
		httpBadRequest(w, err)
		return
	}
	books, err := s.db.CreateBooks(*b)
	if err != nil {
		httpInternalServerError(w, err)
		return
	}
	httpRedirect(w, r, "/book?id="+string(books[0].ID))
}

func (s *Server) putBook(w http.ResponseWriter, r *http.Request) {
	b, err := bookFrom(w, r)
	if err != nil {
		httpBadRequest(w, err)
		return
	}
	var updateImage bool
	var updateImageVal string
	if !ParseFormValue(w, r, "update-image", &updateImageVal, 10) {
		return
	}
	switch updateImageVal {
	case "true":
		updateImage = true
	case "clear":
		updateImage = true
		b.ImageBase64 = ""
	}
	err = s.db.UpdateBook(*b, updateImage)
	if err != nil {
		httpInternalServerError(w, err)
		return
	}
	httpRedirect(w, r, "/book?id="+b.ID)
}

func (s *Server) deleteBook(w http.ResponseWriter, r *http.Request) {
	var id string
	if !ParseFormValue(w, r, "id", &id, 64) {
		return
	}
	if err := s.db.DeleteBook(id); err != nil {
		httpInternalServerError(w, err)
		return
	}
	httpRedirect(w, r, "/")
}

func (s *Server) putAdminPassword(w http.ResponseWriter, r *http.Request) {
	var p1, p2 string
	if !ParseFormValue(w, r, "p1", &p1, 128) || !ParseFormValue(w, r, "p2", &p2, 128) {
		return
	}
	if p1 != p2 {
		err := fmt.Errorf("passwords do not match")
		httpBadRequest(w, err)
		return
	}
	if err := validatePassword(p1); err != nil {
		err := fmt.Errorf("password invalid")
		httpBadRequest(w, err)
		return
	}
	hashedPassword, err := s.ph.Hash([]byte(p1))
	if err != nil {
		httpInternalServerError(w, err)
		return
	}
	if err := s.db.UpdateAdminPassword(string(hashedPassword)); err != nil {
		httpInternalServerError(w, err)
		return
	}
	httpRedirect(w, r, "/")
}

func withAdminPassword(h http.HandlerFunc, db Database, ph PasswordHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hashedPassword, err := db.ReadAdminPassword()
		if err != nil {
			httpInternalServerError(w, err)
			return
		}
		var password string
		if !ParseFormValue(w, r, "p", &password, 128) {
			return
		}
		ok, err := ph.IsCorrectPassword(hashedPassword, []byte(password))
		if err != nil {
			httpInternalServerError(w, err)
			return
		}
		if !ok {
			httpError(w, http.StatusUnauthorized, nil)
			return
		}
		h.ServeHTTP(w, r)
	}
}

func bookFrom(w http.ResponseWriter, r *http.Request) (*book.Book, error) {
	var sb book.StringBook
	if !ParseFormValue(w, r, "id", &sb.ID, 256) ||
		!ParseFormValue(w, r, "title", &sb.Title, 256) ||
		!ParseFormValue(w, r, "author", &sb.Author, 256) ||
		!ParseFormValue(w, r, "description", &sb.Description, 10000) ||
		!ParseFormValue(w, r, "subject", &sb.Subject, 256) ||
		!ParseFormValue(w, r, "dewey-dec-class", &sb.DeweyDecClass, 256) ||
		!ParseFormValue(w, r, "pages", &sb.Pages, 32) ||
		!ParseFormValue(w, r, "publisher", &sb.Publisher, 256) ||
		!ParseFormValue(w, r, "publish-date", &sb.PublishDate, 32) ||
		!ParseFormValue(w, r, "added-date", &sb.AddedDate, 32) ||
		!ParseFormValue(w, r, "ean-isbn-13", &sb.EanIsbn13, 32) ||
		!ParseFormValue(w, r, "upc-isbn-10", &sb.UpcIsbn10, 32) {
		return nil, fmt.Errorf("parse error")
	}
	switch {
	case len(sb.Title) == 0:
		return nil, fmt.Errorf("title required")
	case len(sb.Author) == 0:
		return nil, fmt.Errorf("author required")
	case len(sb.Subject) == 0:
		return nil, fmt.Errorf("subject required")
	case len(sb.AddedDate) == 0:
		return nil, fmt.Errorf("added date required")
	}
	b, err := sb.Book(dateLayout)
	switch {
	case err != nil:
		return nil, fmt.Errorf("parsing book from text: %w", err)
	case b.Pages <= 0:
		return nil, fmt.Errorf("pages required")
	}
	imageBase64, err := parseImage(r)
	if err != nil {
		return nil, err
	}
	b.ImageBase64 = string(imageBase64)
	return b, nil
}

func loadPage[V interface{}](w http.ResponseWriter, r *http.Request, maxRows int, sliceName string, pageLoader func(limit, offset int) ([]V, error)) (data map[string]interface{}, ok bool) {
	var a string
	if !ParseFormValue(w, r, "page", &a, 32) {
		return nil, false
	}
	page := 1
	if len(a) != 0 {
		i, err := strconv.Atoi(a)
		if err != nil {
			err = fmt.Errorf("invalid page: %w", err)
			httpBadRequest(w, err)
			return nil, false
		}
		page = i
	}
	offset := (page - 1) * maxRows
	limit := maxRows + 1
	slice, err := pageLoader(limit, offset)
	if err != nil {
		httpInternalServerError(w, err)
		return nil, false
	}
	data = make(map[string]interface{})
	if len(slice) > maxRows {
		slice = slice[:maxRows]
		data["NextPage"] = page + 1
	}
	data[sliceName] = slice
	return data, true
}

// ParseFormValue reads the value the form by key into dest.
// If the length of the value is longer than maxLength, an error will be written tot he response writer and false is returned.
func ParseFormValue(w http.ResponseWriter, r *http.Request, key string, dest *string, maxLength int) (ok bool) {
	value := r.FormValue(key)
	if len(value) > maxLength {
		err := fmt.Errorf("form value %q too long", key)
		httpError(w, http.StatusRequestEntityTooLarge, err)
		return false
	}
	*dest = value
	return true
}

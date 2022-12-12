package server

import (
	"context"
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
	var filter book.Filter
	if !parseFormValue(w, r, "q", &filter.HeaderPart, 256) {
		return
	}
	if !parseFormValue(w, r, "s", &filter.Subject, 256) {
		return
	}
	pageLoader := func(ctx context.Context, limit, offset int) ([]book.Header, error) {
		return s.db.ReadBookHeaders(ctx, filter, limit, offset)
	}
	if data, ok := loadPage(w, r, s.cfg.MaxRows, "Books", pageLoader); ok {
		data["Filter"] = filter.HeaderPart
		data["Subject"] = filter.Subject
		s.serveTemplate(w, "list", data)
	}
}

func (s *Server) getBook(w http.ResponseWriter, r *http.Request) {
	var id string
	if !parseFormValue(w, r, "id", &id, 64) {
		return
	}
	ctx := r.Context()
	b, err := s.db.ReadBook(ctx, id)
	if err != nil {
		err = fmt.Errorf("reading book: %w", err)
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
		ctx := r.Context()
		b, err := s.db.ReadBook(ctx, id)
		if err != nil {
			err = fmt.Errorf("reading book: %w", err)
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
	ctx := r.Context()
	books, err := s.db.CreateBooks(ctx, *b)
	if err != nil {
		err = fmt.Errorf("creating book: %w", err)
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
	if !parseFormValue(w, r, "update-image", &updateImageVal, 10) {
		return
	}
	switch updateImageVal {
	case "true":
		updateImage = true
	case "clear":
		updateImage = true
		b.ImageBase64 = ""
	}
	ctx := r.Context()
	err = s.db.UpdateBook(ctx, *b, updateImage)
	if err != nil {
		err = fmt.Errorf("updating book: %w", err)
		httpInternalServerError(w, err)
		return
	}
	httpRedirect(w, r, "/book?id="+b.ID)
}

func (s *Server) deleteBook(w http.ResponseWriter, r *http.Request) {
	var id string
	if !parseFormValue(w, r, "id", &id, 64) {
		return
	}
	ctx := r.Context()
	if err := s.db.DeleteBook(ctx, id); err != nil {
		err = fmt.Errorf("deleting book: %w", err)
		httpInternalServerError(w, err)
		return
	}
	httpRedirect(w, r, "/")
}

func (s *Server) putAdminPassword(w http.ResponseWriter, r *http.Request) {
	var p1, p2 string
	if !parseFormValue(w, r, "p1", &p1, 128) || !parseFormValue(w, r, "p2", &p2, 128) {
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
		err = fmt.Errorf("hashing password: %w", err)
		httpInternalServerError(w, err)
		return
	}
	ctx := r.Context()
	if err := s.db.UpdateAdminPassword(ctx, string(hashedPassword)); err != nil {
		err = fmt.Errorf("updating password: %w", err)
		httpInternalServerError(w, err)
		return
	}
	httpRedirect(w, r, "/")
}

func (s *Server) withAdminPassword(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var password string
		if !parseFormValue(w, r, "p", &password, 128) {
			return
		}
		ctx := r.Context()
		hashedPassword, err := s.db.ReadAdminPassword(ctx)
		if err != nil {
			err = fmt.Errorf("reading password: %w", err)
			httpInternalServerError(w, err)
			return
		}
		if len(hashedPassword) == 0 {
			httpError(w, http.StatusServiceUnavailable, fmt.Errorf("password not set"))
			return
		}
		ok, err := s.ph.IsCorrectPassword(hashedPassword, []byte(password))
		if err != nil {
			err = fmt.Errorf("checking password: %w", err)
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
	if !parseFormValue(w, r, "id", &sb.ID, 256) ||
		!parseFormValue(w, r, "title", &sb.Title, 256) ||
		!parseFormValue(w, r, "author", &sb.Author, 256) ||
		!parseFormValue(w, r, "description", &sb.Description, 10000) ||
		!parseFormValue(w, r, "subject", &sb.Subject, 256) ||
		!parseFormValue(w, r, "dewey-dec-class", &sb.DeweyDecClass, 256) ||
		!parseFormValue(w, r, "pages", &sb.Pages, 32) ||
		!parseFormValue(w, r, "publisher", &sb.Publisher, 256) ||
		!parseFormValue(w, r, "publish-date", &sb.PublishDate, 32) ||
		!parseFormValue(w, r, "added-date", &sb.AddedDate, 32) ||
		!parseFormValue(w, r, "ean-isbn-13", &sb.EanIsbn13, 32) ||
		!parseFormValue(w, r, "upc-isbn-10", &sb.UpcIsbn10, 32) {
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

func loadPage[V interface{}](w http.ResponseWriter, r *http.Request, maxRows int, sliceName string, pageLoader func(cxt context.Context, limit, offset int) ([]V, error)) (data map[string]interface{}, ok bool) {
	var a string
	if !parseFormValue(w, r, "page", &a, 32) {
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
	ctx := r.Context()
	slice, err := pageLoader(ctx, limit, offset)
	if err != nil {
		err = fmt.Errorf("loading page: %w", err)
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

// parseFormValue reads the value the form by key into dest.
// If the length of the value is longer than maxLength, an error will be written tot he response writer and false is returned.
func parseFormValue(w http.ResponseWriter, r *http.Request, key string, dest *string, maxLength int) (ok bool) {
	value := r.FormValue(key)
	if len(value) > maxLength {
		err := fmt.Errorf("form value %q too long", key)
		httpError(w, http.StatusRequestEntityTooLarge, err)
		return false
	}
	*dest = value
	return true
}

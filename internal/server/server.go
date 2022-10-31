// Package server runs a web server to display and manage the library
package server

import (
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/csv"
	"github.com/jacobpatterson1549/kuuf-library/internal/server/bcrypt"
)

var (
	//go:embed resources/*
	embedFS embed.FS
	tmpl    = template.Must(template.New("index.html").ParseFS(embedFS, "resources/*"))
)

type (
	Config struct {
		Port          string
		DatabaseURL   string
		BackfillCSV   bool
		AdminPassword string
		MaxRows       int
	}
	Server struct {
		Config
		db Database
		ph PasswordHandler
	}
	PasswordHandler interface {
		Hash(password []byte) (hashedPassword []byte, err error)
		IsCorrectPassword(hashedPassword, password []byte) (ok bool, err error)
	}
	Database interface {
		CreateBooks(books ...book.Book) ([]book.Book, error)
		ReadBooks() ([]book.Header, error)
		ReadBook(id string) (*book.Book, error)
		UpdateBook(b book.Book, updateImage bool) error
		DeleteBook(id string) error
		ReadAdminPassword() (hashedPassword []byte, err error)
		UpdateAdminPassword(hashedPassword string) error
	}
)

func (cfg Config) NewServer() (*Server, error) {
	url, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
	var db Database
	switch s := url.Scheme; s {
	case "csv":
		db, err = csv.NewDatabase()
	default:
		err = fmt.Errorf("unknown database: %q", s)
	}
	if err != nil {
		return nil, fmt.Errorf("creating database: %v", err)
	}
	ph := bcrypt.NewPasswordHandler()
	s := Server{
		Config: cfg,
		db:     db,
		ph:     ph,
	}
	return &s, nil
}

// Run initializes the server and then serves it.
// Initialization reads the config to set the admin password and backfill books from the csv database if desired.
func (s *Server) Run() error {
	if len(s.Config.AdminPassword) != 0 { // setup
		hashedPassword, err := s.ph.Hash([]byte(s.Config.AdminPassword))
		if err != nil {
			return fmt.Errorf("hashing admin password: %w", err)
		}
		if err := s.db.UpdateAdminPassword(string(hashedPassword)); err != nil {
			return fmt.Errorf("setting admin password: %w", err)
		}
	}
	// if _, ok := dao.(csv.Dao); !ok && cfg.BackfillCSV { // setup
	// 	// TODO
	// }
	fmt.Println("Serving resume site at at http://localhost:" + s.Port)
	fmt.Println("Press Ctrl-C to stop")
	return http.ListenAndServe(":"+s.Port, s.mux())
}

func (s *Server) mux() http.Handler {
	static := http.FileServer(http.FS(embedFS))
	m := mux{
		http.MethodGet: map[string]http.HandlerFunc{
			"/":           s.getBooks,
			"/book":       s.getBook,
			"/robots.txt": static.ServeHTTP,
		},
		http.MethodPost: map[string]http.HandlerFunc{
			"/book": s.withAdminPassword(s.postBook),
		},
		http.MethodDelete: map[string]http.HandlerFunc{
			"/book": s.withAdminPassword(s.deleteBook),
		},
		http.MethodPut: map[string]http.HandlerFunc{
			"/book":          s.withAdminPassword(s.putBook),
			"/adminPassword": s.withAdminPassword(s.putAdminPassword),
		},
	}
	day := time.Hour * 24
	return withCacheControl(withContentEncoding(m), day)
}

func (s *Server) getBooks(w http.ResponseWriter, r *http.Request) {
	books, err := s.db.ReadBooks()
	if err != nil {
		httpInternalServerError(w, err)
		return
	}
	serveTemplate(w, "list", books)
}

func (s *Server) getBook(w http.ResponseWriter, r *http.Request) {
	id := bookIDFrom(r)
	b, err := s.db.ReadBook(id)
	if err != nil {
		httpInternalServerError(w, err)
		return
	}
	serveTemplate(w, "book", b)
}

func (s *Server) postBook(w http.ResponseWriter, r *http.Request) {
	b, err := bookFrom(r)
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
	b, err := bookFrom(r)
	var updateImage bool
	switch r.FormValue("updateImage") {
	case "clear":
		updateImage, b.ImageBase64 = true, ""
	case "true":
		updateImage = true
	}
	if err != nil {
		httpBadRequest(w, err)
		return
	}
	if err := s.db.UpdateBook(*b, updateImage); err != nil {
		httpInternalServerError(w, err)
		return
	}
	httpRedirect(w, r, "/book?id="+string(b.ID))
}

func (s *Server) deleteBook(w http.ResponseWriter, r *http.Request) {
	id := bookIDFrom(r)
	if err := s.db.DeleteBook(id); err != nil {
		httpInternalServerError(w, err)
		return
	}
	httpRedirect(w, r, "/")
}

func (s *Server) putAdminPassword(w http.ResponseWriter, r *http.Request) {
	p1 := r.FormValue("p1")
	p2 := r.FormValue("p2")
	if p1 != p2 {
		err := fmt.Errorf("passwords do not match")
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

func serveTemplate(w http.ResponseWriter, name string, data interface{}) {
	type Page struct {
		Name string
		Data interface{}
	}
	p := Page{name, data}
	if err := tmpl.Execute(w, p); err != nil {
		fmt.Println(err)
	}
}

func (s *Server) withAdminPassword(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hashedPassword, err := s.db.ReadAdminPassword()
		if err != nil {
			httpInternalServerError(w, err)
			return
		}
		password := r.FormValue("p")
		ok, err := s.ph.IsCorrectPassword(hashedPassword, []byte(password))
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

func withCacheControl(h http.Handler, d time.Duration) http.HandlerFunc {
	maxAge := "max-age=" + strconv.Itoa(int(d.Seconds()))
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", maxAge)
		h.ServeHTTP(w, r)
	}
}

func withContentEncoding(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.Header.Get("Accept-Encoding"), "gzip"):
			gzw := gzip.NewWriter(w)
			defer gzw.Close()
			wrw := wrappedResponseWriter{
				Writer:         gzw,
				ResponseWriter: w,
			}
			wrw.Header().Set("Content-Encoding", "gzip")
			h.ServeHTTP(wrw, r)
		default:
			h.ServeHTTP(w, r)
		}
	}
}

type wrappedResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (wrw wrappedResponseWriter) Write(p []byte) (n int, err error) {
	return wrw.Writer.Write(p)
}

// mux is http Handler that maps methods to paths to handlers.
type mux map[string]map[string]http.HandlerFunc

// ServeHTTP serves to the path for the method of the request on the handler if such a Handler exists.
func (m mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	methodHandlers, ok := m[r.Method]
	if !ok {
		httpError(w, http.StatusMethodNotAllowed, nil)
		return
	}
	h, ok := methodHandlers[r.URL.Path]
	if !ok {
		httpError(w, http.StatusNotFound, nil)
		return
	}
	h.ServeHTTP(w, r)
}

func httpInternalServerError(w http.ResponseWriter, err error) {
	httpError(w, http.StatusInternalServerError, err)
}

func httpBadRequest(w http.ResponseWriter, err error) {
	httpError(w, http.StatusBadRequest, err)
}

func httpError(w http.ResponseWriter, statusCode int, err error) {
	message := http.StatusText(statusCode)
	if err != nil {
		message += ": " + err.Error()
	}
	http.Error(w, message, statusCode)
}

func httpRedirect(w http.ResponseWriter, r *http.Request, url string) {
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func bookIDFrom(r *http.Request) string {
	return r.FormValue("id")
}

func bookFrom(r *http.Request) (*book.Book, error) {
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
		{&b.DeweyDecimalClassification, "dewey-decimal-classification"},
		{&b.Pages, "pages"},
		{&b.Publisher, "publisher"},
		{&b.PublishDate, "publish-date"},
		{&b.AddedDate, "added-date"},
		{&b.EAN_ISBN13, "ean-isbn13"},
		{&b.UPC_ISBN10, "upc-isbn10"},
		// {&b.ImageBase64,""}, // TODO: handle uploading of images (in image.go)
	}
	for _, f := range fields {
		if err := parseFormValue(f.p, f.key, r); err != nil {
			return nil, err
		}
	}
	return &b, nil
}

func parseFormValue(p interface{}, key string, r *http.Request) error {
	v := r.FormValue(key)
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
		t, err = time.Parse(time.RFC3339, v)
		if err != nil {
			break
		}
		*ptr = t
	}
	if err != nil {
		return fmt.Errorf("parsing key %q (%q) as %T: %v", key, v, p, err)
	}
	return nil
}

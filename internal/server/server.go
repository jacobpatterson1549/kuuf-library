// Package server runs a web server to display and manage the library
package server

import (
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/csv"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/postgres"
	"github.com/jacobpatterson1549/kuuf-library/internal/server/bcrypt"
)

var (
	//go:embed resources/*
	embedFS     embed.FS
	staticFS, _ = fs.Sub(embedFS, "resources")
	tmpl        = template.Must(template.New("index.html").
			Funcs(template.FuncMap{
			"pretty":         prettyInputValue,
			"newDate":        time.Now,
			"dateInputValue": dateInputValue,
		}).
		ParseFS(staticFS, "*"))
)

type (
	Config struct {
		Port          string
		DatabaseURL   string
		BackfillCSV   bool
		UpdateImages  bool
		DumpCSV       bool
		AdminPassword string
		MaxRows       int
		DBTimeoutSec  int
	}
	Server struct {
		Config
		db  Database
		ph  PasswordHandler
		out io.Writer
	}
	PasswordHandler interface {
		Hash(password []byte) (hashedPassword []byte, err error)
		IsCorrectPassword(hashedPassword, password []byte) (ok bool, err error)
	}
	Database interface {
		CreateBooks(books ...book.Book) ([]book.Book, error)
		ReadBookHeaders(f book.Filter, limit, offset int) ([]book.Header, error)
		ReadBook(id string) (*book.Book, error)
		UpdateBook(b book.Book, newID string, updateImage bool) error
		DeleteBook(id string) error
		ReadAdminPassword() (hashedPassword []byte, err error)
		UpdateAdminPassword(hashedPassword string) error
	}
)

func (cfg Config) NewServer(out io.Writer) (*Server, error) {
	url, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
	var db Database
	switch s := url.Scheme; s {
	case "csv":
		db, err = csv.NewDatabase()
	case postgres.DriverName:
		db, err = postgres.NewDatabase(url.String(), time.Second*time.Duration(cfg.DBTimeoutSec))
	default:
		err = fmt.Errorf("unknown database: %q", s)
	}
	if err != nil {
		return nil, fmt.Errorf("creating database: %W", err)
	}
	ph := bcrypt.NewPasswordHandler()
	s := Server{
		Config: cfg,
		db:     db,
		ph:     ph,
		out:    out,
	}
	return &s, nil
}

// Run initializes the server and then serves it.
// Initialization reads the config to set the admin password and backfill books from the csv database if desired.
func (s *Server) Run() error {
	if err := s.setup(); err != nil {
		return fmt.Errorf("setting up server: %w", err)
	}
	fmt.Fprintln(s.out, "Serving resume site at at http://localhost:"+s.Port)
	fmt.Fprintln(s.out, "Press Ctrl-C to stop")
	return http.ListenAndServe(":"+s.Port, s.mux())
}

func (s *Server) initAdminPassword() error {
	hashedPassword, err := s.ph.Hash([]byte(s.Config.AdminPassword))
	if err != nil {
		return fmt.Errorf("hashing admin password: %w", err)
	}
	if err := s.db.UpdateAdminPassword(string(hashedPassword)); err != nil {
		return fmt.Errorf("setting admin password: %w", err)
	}
	return nil
}

func (s *Server) setup() error {
	if len(s.Config.AdminPassword) != 0 {
		if err := s.initAdminPassword(); err != nil {
			return fmt.Errorf("initializing admin password from server configuration: %w", err)
		}
	}
	if _, ok := s.db.(*csv.Database); !ok && s.BackfillCSV {
		if err := s.backfillCSV(); err != nil {
			return fmt.Errorf("backfilling database from internal CSV file: %w", err)
		}
	}
	if s.UpdateImages || s.DumpCSV {
		if err := s.updateImages(); err != nil {
			return fmt.Errorf("updating images / dumping csv;: %w", err)
		}
	}
	return nil
}

func (s *Server) backfillCSV() error {
	src, err := csv.NewDatabase()
	if err != nil {
		return fmt.Errorf("loading csv database: %w", err)
	}
	books := src.Books
	if _, err := s.db.CreateBooks(books...); err != nil {
		return fmt.Errorf("creating books: %w", err)
	}
	return nil
}

func (s *Server) updateImages() error {
	d := csv.NewDump(s.out)
	offset := 0
	for {
		headers, err := s.db.ReadBookHeaders(nil, s.MaxRows+1, offset)
		if err != nil {
			return fmt.Errorf("reading books at offset %v: %w", offset, err)
		}
		hasMore := len(headers) > s.MaxRows
		if hasMore {
			headers = headers[:s.MaxRows]
		}
		for _, h := range headers {
			b, err := s.db.ReadBook(h.ID)
			if err != nil {
				return fmt.Errorf("reading book %q: %w", h.ID, err)
			}
			if s.UpdateImages {
				if len(b.ImageBase64) == 0 {
					continue
				}
				imageBase64, err := updateImage(b.ImageBase64, b.ID)
				if err != nil {
					return fmt.Errorf("updating image for book %q: %w", b.ID, err)
				}
				b.ImageBase64 = string(imageBase64)
				if err := s.db.UpdateBook(*b, b.ID, true); err != nil {
					return fmt.Errorf("writing updated image to db for book %q: %w", b.ID, err)
				}
			}
			if s.DumpCSV {
				d.Write(*b)
			}
		}
		if !hasMore {
			return nil
		}
		offset += s.MaxRows
	}
}

func (s *Server) mux() http.Handler {
	static := http.FileServer(http.FS(staticFS))
	m := mux{
		http.MethodGet: map[string]http.HandlerFunc{
			"/":           s.getBookHeaders,
			"/book":       s.getBook,
			"/admin":      s.getAdmin,
			"/robots.txt": static.ServeHTTP,
		},
		http.MethodPost: map[string]http.HandlerFunc{
			"/book/create":  s.postBook,
			"/book/delete":  s.deleteBook,
			"/book/update":  s.putBook,
			"/admin/update": s.putAdminPassword,
		},
	}
	authenticatedMethods := []string{
		http.MethodPost,
	}
	for _, n := range authenticatedMethods {
		for p, h := range m[n] {
			m[n][p] = s.withAdminPassword(h)
		}
	}
	day := time.Hour * 24
	return withCacheControl(withContentEncoding(m), day)
}

func (s *Server) getBookHeaders(w http.ResponseWriter, r *http.Request) {
	var page int
	if a := r.FormValue("page"); len(a) != 0 {
		i, err := strconv.Atoi(a)
		if err != nil {
			err = fmt.Errorf("invalid page: %w", err)
			httpBadRequest(w, err)
			return
		}
		page = i
	}
	q := r.FormValue("q")
	filter, err := book.NewFilter(q)
	if err != nil {
		httpBadRequest(w, err)
		return
	}
	offset := page * s.MaxRows
	limit := s.MaxRows + 1
	books, err := s.db.ReadBookHeaders(*filter, limit, offset)
	if err != nil {
		httpInternalServerError(w, err)
		return
	}
	data := struct {
		Books    []book.Header
		Filter   string
		NextPage int
	}{
		Filter: q,
		Books:  books,
	}
	if len(data.Books) > s.MaxRows {
		data.Books = data.Books[1:]
		data.NextPage = page + 1
	}
	s.serveTemplate(w, "list", data)
}

func (s *Server) getBook(w http.ResponseWriter, r *http.Request) {
	id := bookIDFrom(r)
	b, err := s.db.ReadBook(id)
	if err != nil {
		httpInternalServerError(w, err)
		return
	}
	s.serveTemplate(w, "book", b)
}

func (s *Server) getAdmin(w http.ResponseWriter, r *http.Request) {
	var data interface{}
	hasID := r.URL.Query().Has("id")
	if hasID {
		id := r.URL.Query().Get("id")
		b, err := s.db.ReadBook(id)
		if err != nil {
			httpInternalServerError(w, err)
			return
		}
		data = b
	}
	s.serveTemplate(w, "admin", data)
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
	if err != nil {
		httpBadRequest(w, err)
		return
	}
	newID := book.NewID()
	var updateImage bool
	switch r.FormValue("update-image") {
	case "true":
		updateImage = true
	case "clear":
		updateImage = true
		b.ImageBase64 = ""
	}
	err = s.db.UpdateBook(*b, newID, updateImage)
	if err != nil {
		httpInternalServerError(w, err)
		return
	}
	httpRedirect(w, r, "/book?id="+newID)
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

func (s *Server) serveTemplate(w http.ResponseWriter, name string, data interface{}) {
	type Page struct {
		Name string
		Data interface{}
	}
	p := Page{name, data}
	if err := tmpl.Execute(w, p); err != nil {
		fmt.Fprintln(s.out, err)
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
		p        interface{}
		key      string
		required bool
	}{
		{&b.ID, "id", false},
		{&b.Title, "title", true},
		{&b.Author, "author", true},
		{&b.Description, "description", false},
		{&b.Subject, "subject", true},
		{&b.DeweyDecClass, "dewey-dec-class", false},
		{&b.Pages, "pages", true},
		{&b.Publisher, "publisher", false},
		{&b.PublishDate, "publish-date", false},
		{&b.AddedDate, "added-date", true},
		{&b.EAN_ISBN13, "ean-isbn-13", false},
		{&b.UPC_ISBN10, "upc-isbn-10", false},
	}
	for _, f := range fields {
		if err := parseFormValue(f.p, f.key, f.required, r); err != nil {
			return nil, err
		}
	}
	imageBase64, err := parseImage(r)
	if err != nil {
		return nil, err
	}
	b.ImageBase64 = string(imageBase64)
	return &b, nil
}

func parseFormValue(p interface{}, key string, required bool, r *http.Request) error {
	v := r.FormValue(key)
	var err error
	if len(v) == 0 {
		if required {
			err = fmt.Errorf("value not set")
		}
	} else {
		switch ptr := p.(type) {
		case *string:

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
			t, err = time.Parse(dateLayout, v)
			// TODO: Look into normalizing the logic a similar function in csv.Database.
			if err != nil {
				break
			}
			*ptr = t
		}
	}
	if err != nil {
		return fmt.Errorf("parsing key %q (%q) as %T: %w", key, v, p, err)
	}
	return nil
}

const dateLayout = "2006-01-02"

func dateInputValue(i interface{}) string {
	switch t := i.(type) {
	case time.Time:
		return t.Format(dateLayout)
	}
	return ""
}

func prettyInputValue(i interface{}) interface{} {
	if i == nil {
		return ""
	}
	return i
}

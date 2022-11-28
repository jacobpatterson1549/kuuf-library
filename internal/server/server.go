// Package server runs a web server to display and manage the library
package server

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/csv"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/mongo"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/postgres"
	"github.com/jacobpatterson1549/kuuf-library/internal/server/bcrypt"
)

var (
	//go:embed resources/favicon.svg
	faviconSVG string
	//go:embed resources/library.csv
	libraryCSV string
	//go:embed resources/*
	embedFS     embed.FS
	staticFS, _ = fs.Sub(embedFS, "resources")
)

const (
	dateLayout         = book.HyphenatedYYYYMMDD
	validPasswordRunes = "`" + `!"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\]^_abcdefghijklmnopqrstuvwxyz{|}~`
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
		PostLimitSec  int
		PostMaxBurst  int
	}
	Server struct {
		Config
		favicon string
		tmpl    *template.Template
		db      Database
		ph      PasswordHandler
		out     io.Writer
	}
	PasswordHandler interface {
		Hash(password []byte) (hashedPassword []byte, err error)
		IsCorrectPassword(hashedPassword, password []byte) (ok bool, err error)
	}
	Database interface {
		CreateBooks(books ...book.Book) ([]book.Book, error)
		ReadBookSubjects(limit, offset int) ([]book.Subject, error)
		ReadBookHeaders(f book.Filter, limit, offset int) ([]book.Header, error)
		ReadBook(id string) (*book.Book, error)
		UpdateBook(b book.Book, updateImage bool) error
		DeleteBook(id string) error
		ReadAdminPassword() (hashedPassword []byte, err error)
		UpdateAdminPassword(hashedPassword string) error
	}
)

func (cfg Config) NewServer(out io.Writer) (*Server, error) {
	db, err := cfg.createDatabase()
	if err != nil {
		return nil, fmt.Errorf("creating database: %W", err)
	}
	favicon := faviconBase64()
	ph := bcrypt.NewPasswordHandler()
	tmpl := parseTemplate()
	s := Server{
		Config:  cfg,
		favicon: favicon,
		tmpl:    tmpl,
		db:      db,
		ph:      ph,
		out:     out,
	}
	return &s, nil
}

// Run initializes the server and then serves it.
// Initialization reads the config to set the admin password and backfill books from the csv database if desired.
func (s *Server) Run() error {
	if err := s.Config.setup(s.db, s.ph, s.out); err != nil {
		return fmt.Errorf("setting up server: %w", err)
	}
	fmt.Fprintf(s.out, "Using database: %T.\n", s.db)
	fmt.Fprintf(s.out, "Serving library at at http://localhost:%v\n", s.Port)
	fmt.Fprintf(s.out, "Press Ctrl-C to stop.\n")
	lim := s.postRateLimiter()
	return http.ListenAndServe(":"+s.Port, s.mux(lim))
}

func (cfg Config) createDatabase() (Database, error) {
	url, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
	switch s := url.Scheme; s {
	case "csv":
		return embeddedCSVDatabase()
	case "mongodb+srv":
		return mongo.NewDatabase(url.String(), cfg.queryTimeout())
	case postgres.DriverName:
		return postgres.NewDatabase(url.String(), cfg.queryTimeout())
	default:
		return nil, fmt.Errorf("unknown database: %q", s)
	}
}

func embeddedCSVDatabase() (*csv.Database, error) {
	r := strings.NewReader(libraryCSV)
	return csv.NewDatabase(r)
}

func parseTemplate() *template.Template {
	funcs := template.FuncMap{
		"pretty":         prettyInputValue,
		"newDate":        time.Now,
		"dateInputValue": dateInputValue,
	}
	return template.Must(template.New("index.html").
		Funcs(funcs).
		ParseFS(staticFS, "*"))
}

func (s *Server) mux(postRateLimiter rateLimiter) http.Handler {
	static := http.FileServer(http.FS(staticFS))
	m := mux{
		http.MethodGet: map[string]http.HandlerFunc{
			"/":           s.getBookSubjects,
			"/list":       s.getBookHeaders,
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
			h1 := withAdminPassword(h, s.db, s.ph)
			h2 := withRateLimiter(h1, postRateLimiter)
			m[n][p] = h2
		}
	}
	duration := time.Hour * 24 // update message in admin.html when updating cache age
	h1 := withContentEncoding(m)
	h := withCacheControl(h1, duration)
	return h
}

func (s *Server) serveTemplate(w http.ResponseWriter, name string, data interface{}) {
	type Page struct {
		Favicon string
		Name    string
		Data    interface{}
	}
	p := Page{s.favicon, name, data}
	if err := s.tmpl.Execute(w, p); err != nil {
		fmt.Fprintln(s.out, err)
	}
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

func dateInputValue(t time.Time) string {
	return t.Format(string(dateLayout))
}

func prettyInputValue(i interface{}) interface{} {
	if s, ok := i.(string); ok {
		return template.HTMLEscapeString(s)
	}
	return i
}

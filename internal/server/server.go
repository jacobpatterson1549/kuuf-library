// Package server runs a web server to display and manage the library
package server

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/csv"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/mongo"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/sql"
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
		cfg      Config
		favicon  string
		tmpl     *template.Template
		staticFS fs.FS
		db       database
		ph       passwordHandler
		pv       passwordValidator
		out      io.Writer
	}
	passwordHandler interface {
		Hash(password []byte) (hashedPassword []byte, err error)
		IsCorrectPassword(hashedPassword, password []byte) (ok bool, err error)
	}
	database interface {
		CreateBooks(ctx context.Context, books ...book.Book) ([]book.Book, error)
		ReadBookSubjects(ctx context.Context, limit, offset int) ([]book.Subject, error)
		ReadBookHeaders(ctx context.Context, f book.Filter, limit, offset int) ([]book.Header, error)
		ReadBook(ctx context.Context, id string) (*book.Book, error)
		UpdateBook(ctx context.Context, b book.Book, updateImage bool) error
		DeleteBook(ctx context.Context, id string) error
		ReadAdminPassword(ctx context.Context) (hashedPassword []byte, err error)
		UpdateAdminPassword(ctx context.Context, hashedPassword string) error
	}
	// page is sent to templates
	page struct {
		Favicon string
		Name    string
		Data    interface{}
	}
	passwordValidatorConfig struct {
		minLength  int
		validRunes string
	}
	passwordValidator struct {
		passwordValidatorConfig
		validRunes map[rune]struct{}
	}
)

// NewServer creates and initializes a new server.
// Initialization reads the config to set the admin password and backfill books from the csv database if desired.
func (cfg Config) NewServer(ctx context.Context, out io.Writer) (*Server, error) {
	db, err := cfg.createDatabase(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating database: %W", err)
	}
	favicon := faviconBase64()
	ph := bcrypt.NewPasswordHandler()
	fsys := staticFS
	tmpl := parseTemplate(fsys)
	pvc := passwordValidatorConfig{
		minLength:  8,
		validRunes: validPasswordRunes,
	}
	pv := pvc.NewPasswordValidator()
	if err := cfg.setup(ctx, db, ph, pv, out); err != nil {
		return nil, fmt.Errorf("setting up server: %w", err)
	}
	s := Server{
		cfg:      cfg,
		favicon:  favicon,
		tmpl:     tmpl,
		staticFS: fsys,
		db:       db,
		ph:       ph,
		pv:       pv,
		out:      out,
	}
	return &s, nil
}

func (s *Server) RunSync() error {
	dbScheme, _, _ := strings.Cut(s.cfg.DatabaseURL, ":")
	fmt.Fprintf(s.out, "Using database: %q (%T).\n", dbScheme, s.db)
	fmt.Fprintf(s.out, "Serving library at at http://localhost:%v\n", s.cfg.Port)
	fmt.Fprintf(s.out, "Press Ctrl-C to stop.\n")
	lim := s.cfg.postRateLimiter()
	addr := ":" + s.cfg.Port
	handler := s.mux(lim)
	return http.ListenAndServe(addr, handler) // BLOCKING
}

func (cfg Config) createDatabase(ctx context.Context) (database, error) {
	switch s := cfg.databaseScheme(); s {
	case "csv":
		return embeddedCSVDatabase()
	case "mongodb+srv":
		return mongo.NewDatabase(ctx, cfg.DatabaseURL)
	case "postgres":
		return sql.NewDatabase(ctx, s, cfg.DatabaseURL)
	case "file":
		return sql.NewDatabase(ctx, "sqlite3", cfg.DatabaseURL)
	default:
		return nil, fmt.Errorf("unknown database: %q", s)
	}
}

func embeddedCSVDatabase() (database, error) {
	r := strings.NewReader(libraryCSV)
	d, err := csv.NewDatabase(r)
	if err != nil {
		return readOnlyDatabase{}, fmt.Errorf("initializing csv database: %w", err)
	}
	d2 := readOnlyDatabase{
		ReadBookSubjectsFunc: func(ctx context.Context, limit, offset int) ([]book.Subject, error) {
			return d.ReadBookSubjects(limit, offset)
		},
		ReadBookHeadersFunc: func(ctx context.Context, filter book.Filter, limit, offset int) ([]book.Header, error) {
			return d.ReadBookHeaders(filter, limit, offset)
		},
		ReadBookFunc: func(ctx context.Context, id string) (*book.Book, error) {
			return d.ReadBook(id)
		},
	}
	d3 := allBooksDatabase{
		database: d2,
		AllBooksFunc: func() ([]book.Book, error) {
			return d.Books, nil
		},
	}
	return d3, nil
}

func parseTemplate(fsys fs.FS) *template.Template {
	funcs := template.FuncMap{
		"pretty":         prettyInputValue,
		"newDate":        time.Now,
		"dateInputValue": dateInputValue,
	}
	return template.Must(template.New("index.html").
		Funcs(funcs).
		ParseFS(fsys, "*"))
}

func (s *Server) mux(postRateLimiter rateLimiter) http.Handler {
	static := http.FileServer(http.FS(s.staticFS))
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
			h1 := s.withAdminPassword(h)
			h2 := withRateLimiter(h1, postRateLimiter)
			m[n][p] = h2
		}
	}
	duration := time.Hour * 24 // update message in admin.html when updating cache age
	queryTimeout := s.cfg.queryTimeout()
	h := withContentEncoding(m)
	h = withCacheControl(h, duration)
	h = withContextTimeout(h, queryTimeout)
	return h
}

func (s *Server) serveTemplate(w http.ResponseWriter, name string, data interface{}) {
	p := page{s.favicon, name, data}
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

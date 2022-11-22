// Package server runs a web server to display and manage the library
package server

import (
	"bufio"
	"compress/gzip"
	"embed"
	"encoding/base64"
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
	"github.com/jacobpatterson1549/kuuf-library/internal/db/mongo"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/postgres"
	"github.com/jacobpatterson1549/kuuf-library/internal/server/bcrypt"
	"golang.org/x/time/rate"
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
		PostLimitSec  int
		PostMaxBurst  int
	}
	Server struct {
		Config
		favicon string
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
	url, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}
	var db Database
	switch s := url.Scheme; s {
	case "csv":
		db, err = csv.NewDatabase()
	case "mongodb+srv":
		db, err = mongo.NewDatabase(url.String(), cfg.queryTimeout())
	case postgres.DriverName:
		db, err = postgres.NewDatabase(url.String(), cfg.queryTimeout())
	default:
		err = fmt.Errorf("unknown database: %q", s)
	}
	if err != nil {
		return nil, fmt.Errorf("creating database: %W", err)
	}
	favicon, err := faviconBase64()
	if err != nil {
		return nil, fmt.Errorf("loading favicon: %w", err)
	}
	ph := bcrypt.NewPasswordHandler()
	s := Server{
		Config:  cfg,
		favicon: string(favicon),
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
	fmt.Fprintln(s.out, "Serving resume site at at http://localhost:"+s.Port)
	fmt.Fprintln(s.out, "Press Ctrl-C to stop")
	return http.ListenAndServe(":"+s.Port, s.mux())
}

func (cfg Config) queryTimeout() time.Duration {
	return time.Second * time.Duration(cfg.DBTimeoutSec)
}

func (cfg Config) setup(db Database, ph PasswordHandler, out io.Writer) error {
	if len(cfg.AdminPassword) != 0 {
		if err := cfg.initAdminPassword(db, ph); err != nil {
			return fmt.Errorf("initializing admin password from server configuration: %w", err)
		}
	}
	if cfg.BackfillCSV {
		if err := cfg.backfillCSV(db); err != nil {
			return fmt.Errorf("backfilling database from internal CSV file: %w", err)
		}
	}
	if cfg.UpdateImages || cfg.DumpCSV {
		if err := cfg.updateImages(db, out); err != nil {
			return fmt.Errorf("updating images / dumping csv;: %w", err)
		}
	}
	return nil
}

func (cfg Config) initAdminPassword(db Database, ph PasswordHandler) error {
	hashedPassword, err := ph.Hash([]byte(cfg.AdminPassword))
	if err != nil {
		return fmt.Errorf("hashing admin password: %w", err)
	}
	if err := db.UpdateAdminPassword(string(hashedPassword)); err != nil {
		return fmt.Errorf("setting admin password: %w", err)
	}
	return nil
}

func (cfg Config) backfillCSV(db Database) error {
	src, err := csv.NewDatabase()
	if err != nil {
		return fmt.Errorf("loading csv database: %w", err)
	}
	books := src.Books
	if _, err := db.CreateBooks(books...); err != nil {
		return fmt.Errorf("creating books: %w", err)
	}
	return nil
}

func (cfg Config) updateImages(db Database, out io.Writer) error {
	d := csv.NewDump(out)
	offset := 0
	for {
		headers, err := db.ReadBookHeaders(book.Filter{}, cfg.MaxRows+1, offset)
		if err != nil {
			return fmt.Errorf("reading books at offset %v: %w", offset, err)
		}
		hasMore := len(headers) > cfg.MaxRows
		if hasMore {
			headers = headers[:cfg.MaxRows]
		}
		for _, h := range headers {
			if err := cfg.updateImage(h, db, *d); err != nil {
				return err
			}
		}
		if !hasMore {
			return nil
		}
		offset += cfg.MaxRows
	}
}

func (cfg Config) updateImage(h book.Header, db Database, d csv.Dump) error {
	b, err := db.ReadBook(h.ID)
	if err != nil {
		return fmt.Errorf("reading book %q: %w", h.ID, err)
	}
	if cfg.UpdateImages && imageNeedsUpdating(b.ImageBase64) {
		imageBase64, err := updateImage(b.ImageBase64, b.ID)
		if err != nil {
			return fmt.Errorf("updating image for book %q: %w", b.ID, err)
		}
		b.ImageBase64 = string(imageBase64)
		if err := db.UpdateBook(*b, true); err != nil {
			return fmt.Errorf("writing updated image to db for book %q: %w", b.ID, err)
		}
	}
	if cfg.DumpCSV {
		d.Write(*b)
	}
	return nil
}

func (s *Server) mux() http.Handler {
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
	r := 1 / rate.Limit(s.PostLimitSec)
	lim := rate.NewLimiter(r, s.PostMaxBurst)
	for _, n := range authenticatedMethods {
		for p, h := range m[n] {
			m[n][p] = withRateLimiter(s.withAdminPassword(h), lim)
		}
	}
	day := time.Hour * 24
	return withCacheControl(withContentEncoding(m), day) // update message in admin.html when updating cache age
}

func (s *Server) getBookSubjects(w http.ResponseWriter, r *http.Request) {
	if data, ok := loadPage(w, r, s.MaxRows, "Subjects", s.db.ReadBookSubjects); ok {
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
	filter, err := book.NewFilter(headerParts, subject)
	if err != nil {
		httpBadRequest(w, err)
		return
	}
	pageLoader := func(limit, offset int) ([]book.Header, error) {
		return s.db.ReadBookHeaders(*filter, limit, offset)
	}
	if data, ok := loadPage(w, r, s.MaxRows, "Books", pageLoader); ok {
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
		ValidPasswordRunes: validPasswordRunes,
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

func validatePassword(p string) error {
	if len(p) < 8 {
		return fmt.Errorf("password too short")
	}
	m := make(map[rune]struct{}, len(validPasswordRunes))
	for _, r := range validPasswordRunes {
		m[r] = struct{}{}
	}
	for _, r := range p {
		if _, ok := m[r]; !ok {
			return fmt.Errorf("password contains characters that are not allowed")
		}
	}
	return nil
}

func (s *Server) serveTemplate(w http.ResponseWriter, name string, data interface{}) {
	type Page struct {
		Favicon string
		Name    string
		Data    interface{}
	}
	p := Page{s.favicon, name, data}
	if err := tmpl.Execute(w, p); err != nil {
		fmt.Fprintln(s.out, err)
	}
}

func faviconBase64() ([]byte, error) {
	f, err := staticFS.Open("favicon.svg")
	if err != nil {
		return nil, err
	}
	br := bufio.NewReader(f)
	var sb strings.Builder
	enc := base64.NewEncoder(base64.StdEncoding, &sb)
	if _, err := br.WriteTo(enc); err != nil {
		return nil, err
	}
	enc.Close()
	return []byte(sb.String()), nil
}

func withRateLimiter(h http.HandlerFunc, lim *rate.Limiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !lim.Allow() {
			err := fmt.Errorf("too many POSTS to server")
			httpError(w, http.StatusTooManyRequests, err)
			return
		}
		h.ServeHTTP(w, r)
	}
}

func (s *Server) withAdminPassword(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hashedPassword, err := s.db.ReadAdminPassword()
		if err != nil {
			httpInternalServerError(w, err)
			return
		}
		var password string
		if !ParseFormValue(w, r, "p", &password, 128) {
			return
		}
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

// withCacheControl adds a cache-control to GET requests that are not to edit a book
func withCacheControl(h http.Handler, d time.Duration) http.HandlerFunc {
	maxAge := "max-age=" + strconv.Itoa(int(d.Seconds()))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && (r.URL.Path != "/admin" || len(r.URL.Query()["book-id"]) == 0) {
			w.Header().Add("Cache-Control", maxAge)
		}
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
		!ParseFormValue(w, r, "ean-isbn-13", &sb.EAN_ISBN13, 32) ||
		!ParseFormValue(w, r, "upc-isbn-10", &sb.UPC_ISBN10, 32) {
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

const dateLayout = book.HyphenatedYYYYMMDD
const validPasswordRunes = "`" + `!"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\]^_abcdefghijklmnopqrstuvwxyz{|}~`

func dateInputValue(i interface{}) string {
	switch t := i.(type) {
	case time.Time:
		return t.Format(string(dateLayout))
	}
	return ""
}

func prettyInputValue(i interface{}) interface{} {
	if i == nil {
		return ""
	}
	if s, ok := i.(string); ok {
		return template.HTMLEscapeString(s)
	}
	return i
}

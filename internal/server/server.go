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
	"github.com/jacobpatterson1549/kuuf-library/internal/db/postgres"
	"github.com/jacobpatterson1549/kuuf-library/internal/server/bcrypt"
)

var (
	//go:embed resources/*
	embedFS embed.FS
	tmpl    = template.Must(template.New("index.html").
		Funcs(template.FuncMap{
			"pretty":         prettyInputValue,
			"dateInputValue": dateInputValue,
		}).
		ParseFS(embedFS, "resources/*"))
)

type (
	Config struct {
		Port          string
		DatabaseURL   string
		BackfillCSV   bool
		AdminPassword string
		MaxRows       int
		DBTimeoutSec  int
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
		ReadBooks() ([]*book.Header, error) // TODO: think about adding Header interface here to avoid using array of pointers
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
	case postgres.DriverName:
		db, err = postgres.NewDatabase(url.String(), time.Second*time.Duration(cfg.DBTimeoutSec))
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
		if err := s.initAdminPassword(); err != nil {
			return fmt.Errorf("initializing admin password from server configuration: %v", err)
		}
	}
	if _, ok := s.db.(*csv.Database); !ok && s.BackfillCSV { // setup
		if err := s.backfillCSV(); err != nil {
			return fmt.Errorf("backfilling database from internal CSV file: %v", err)
		}
	}
	fmt.Println("Serving resume site at at http://localhost:" + s.Port)
	fmt.Println("Press Ctrl-C to stop")
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

func (s *Server) backfillCSV() error {
	src, err := csv.NewDatabase()
	if err != nil {
		return fmt.Errorf("loading csv database: %v", err)
	}
	books := src.Books
	if _, err := s.db.CreateBooks(books...); err != nil {
		return fmt.Errorf("creating books: %v", err)
	}
	return nil
}

func (s *Server) mux() http.Handler {
	static := http.FileServer(http.FS(embedFS))
	m := mux{
		http.MethodGet: map[string]http.HandlerFunc{
			"/":           s.getBooks,
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
	// TODO: figure out how to handle cache-control after updating.  Maybe books should get new ids when updating?
	// TODO: blacklist /admin from robots.txt
	// day := time.Hour * 24
	// return withCacheControl(withContentEncoding(m), day)
	return withContentEncoding(m)
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
	serveTemplate(w, "admin", data)
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
	updateImage := r.FormValue("update-image") == "true"
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
		p        interface{}
		key      string
		required bool
	}{
		{&b.ID, "id", false}, // TODO: handle create vs update
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
		// {&b.ImageBase64, "image", false}, // TODO
	}
	for _, f := range fields {
		if err := parseFormValue(f.p, f.key, f.required, r); err != nil {
			return nil, err
		}
	}
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
		return fmt.Errorf("parsing key %q (%q) as %T: %v", key, v, p, err)
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

// // webP should be used in the kuuf-library server to encode uploaded jpg/png images
// func webP(b []byte, title string) ([]byte, error) {
// 	// return b, nil
// 	// TODO: stream b to cwebp command.  As of 2022, this is not possible.
// 	f, err := os.CreateTemp("", title)
// 	if err != nil {
// 		return nil, fmt.Errorf("creating temp file: %v", err)
// 	}
// 	n := f.Name()
// 	defer os.Remove(n)
// 	cmd := exec.Command("cwebp", n, "-o", "-")
// 	b2, err := cmd.Output()
// 	if err != nil {
// 		return nil, fmt.Errorf("running cwebp: %v", err)
// 	}
// 	all := base64.URLEncoding.EncodeToString(b2)
// 	// r := base64.NewDecoder(base64.URLEncoding, stdout)
// 	// all, err := io.ReadAll(r)
// 	// if err != nil {
// 	// 	return nil, fmt.Errorf("decoding webp to base64: %v", err)
// 	// }
// 	return []byte(all), nil
// }

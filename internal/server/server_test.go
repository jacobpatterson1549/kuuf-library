package server

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/csv"
	"golang.org/x/time/rate"
)

func TestNewServer(t *testing.T) {
	tests := []struct {
		name   string
		cfg    Config
		wantOk bool
	}{
		{
			name: "empty database url",
		},
		{
			name: "database url parse error",
			cfg: Config{
				DatabaseURL: "csv ://",
			},
		},
		{
			name: "unknown database url",
			cfg: Config{
				DatabaseURL: "oracle://some_db99:1234/ORCL",
			},
		},
		{
			name: "csv",
			cfg: Config{
				DatabaseURL: "csv://",
				MaxRows:     42,
			},
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sb strings.Builder
			got, err := test.cfg.NewServer(&sb)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case got.Config != test.cfg:
				t.Errorf("configs not equal: \n wanted: %v \n got:     %v", got.Config, test.cfg)
			case got.db == nil:
				t.Errorf("database not set")
			case got.ph == nil:
				t.Errorf("password handler not set")
			case got.out != &sb:
				t.Errorf("output writers not equal: \n wanted: %v \n got:     %v", got.out, &sb)
			}
		})
	}
}

func TestMux(t *testing.T) {
	s := Server{
		db: mockDatabase{
			readBookSubjectsFunc: func(limit, offset int) ([]book.Subject, error) {
				return nil, nil
			},
			readBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
				return nil, nil
			},
			readBookFunc: func(id string) (*book.Book, error) {
				return new(book.Book), nil
			},
		},
	}
	h := s.mux()
	tests := []struct {
		name     string
		method   string
		url      string
		wantCode int
	}{
		{"bad method", "patch", "/", 405},
		{"subjects", "GET", "/", 200},
		{"list", "GET", "/list", 200},
		{"book", "GET", "/book", 200},
		{"admin", "GET", "/admin", 200},
		{"robots.txt", "GET", "/robots.txt", 200},
		{"not found", "GET", "/bad.html", 404},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			s.out = &buf
			r := httptest.NewRequest(test.method, test.url, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if want, got := test.wantCode, w.Code; want != got {
				t.Errorf("wanted %v, got %v", want, got)
			}
			if buf.Len() != 0 {
				t.Errorf("unwanted log: %q", buf.String())
			}
		})
	}
}

func TestWithCacheControl(t *testing.T) {
	msg := "OK_1549"
	h1 := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(msg))
	}
	tests := []struct {
		name             string
		wantCacheControl bool
		r                *http.Request
	}{
		{"subjects get", true, httptest.NewRequest("GET", "/", nil)},
		{"book get", true, httptest.NewRequest("GET", "/book?id=existing", nil)},
		{"add book get", true, httptest.NewRequest("GET", "/admin", nil)},
		{"edit book get", false, httptest.NewRequest("GET", "/admin?book-id=existing", nil)},
		{"add book post", false, httptest.NewRequest("POST", "/admin", nil)},
		{"list", true, httptest.NewRequest("GET", "/list", nil)},
		{"list  search", true, httptest.NewRequest("GET", "/list?q=search", nil)},
		{"book update", false, httptest.NewRequest("POST", "/book?id=existing", nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h2 := withCacheControl(http.HandlerFunc(h1), time.Minute)
			w := httptest.NewRecorder()
			h2.ServeHTTP(w, test.r)
			switch {
			case w.Body.String() != msg:
				t.Errorf("inner handler not run: wanted body to be %q, got %q", w.Body.String(), msg)
			default:
				want := "max-age=60"
				got := w.Result().Header.Get("Cache-Control")
				gotCacheControl := want == got
				if test.wantCacheControl != gotCacheControl {
					t.Errorf("wanted cache control: %v, got: %q", test.wantCacheControl, got)
				}
			}
		})
	}
}

func TestWithContentEncoding(t *testing.T) {
	msg := "OK_gzip"
	h1 := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(msg))
	}
	h2 := withContentEncoding(http.HandlerFunc(h1))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("", "/", nil)
	r.Header.Add("Accept-Encoding", "gzip, deflate, br")
	h2.ServeHTTP(w, r)
	got := w.Result()
	switch {
	case got.Header.Get("Content-Encoding") != "gzip":
		t.Errorf("wanted gzip Content-Encoding, got: %q", got.Header.Get("Content-Encoding"))
	default:
		r, err := gzip.NewReader(got.Body)
		if err != nil {
			t.Fatalf("creating gzip reader: %v", err)
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("reading gzip encoded message: %v", err)
		}
		if want, got := msg, string(b); want != got {
			t.Errorf("body not encoded as desired: wanted %q, got %q", want, got)
		}
	}
}

func TestWithRateLimiter(t *testing.T) {
	tests := []struct {
		name        string
		wantCode    int
		lim         *rate.Limiter
		numRequests int
	}{
		{"zero burst", 429, rate.NewLimiter(1, 0), 1},
		{"first allowed", 200, rate.NewLimiter(1, 1), 1},
		{"fourth allowed", 429, rate.NewLimiter(1, 4), 5},
		{"fifth not allowed", 429, rate.NewLimiter(1, 4), 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h1 := func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			}
			var w *httptest.ResponseRecorder
			h2 := withRateLimiter(h1, test.lim)
			r := httptest.NewRequest("POST", "/admin", nil)
			for i := 0; i < test.numRequests; i++ {
				w = httptest.NewRecorder()
				h2.ServeHTTP(w, r)
			}
			if want, got := test.wantCode, w.Code; want != got {
				t.Errorf("status codes not equal: wanted %v, got %v", want, got)
			}
		})
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		name             string
		url              string
		maxRows          int
		readBook         func(id string) (*book.Book, error)
		readBookSubjects func(limit, offset int) ([]book.Subject, error)
		readBookHeaders  func(f book.Filter, limit, offset int) ([]book.Header, error)
		wantCode         int
		wantData         []string
		unwantedData     []string
	}{
		{
			name:     "admin: MissingKeyZero",
			url:      "/admin",
			wantData: []string{`name="title" value="" required`},
			wantCode: 200,
		},
		{
			name:    "subject: with space",
			url:     "/",
			maxRows: 1,
			wantData: []string{
				`tall buildings`, // display value
				`tall+buildings`, // href
			},
			readBookSubjects: func(limit, offset int) ([]book.Subject, error) {
				return []book.Subject{{Name: "tall buildings"}}, nil
			},
			wantCode: 200,
		},
		{
			name:     "admin: TitleContainsQuote",
			url:      "/admin?book-id=wow",
			wantData: []string{`name="title" value="&#34;Wow,&#34; A Memoir" required`},
			readBook: func(id string) (*book.Book, error) {
				b := book.Book{
					Header: book.Header{
						Title: `"Wow," A Memoir`,
					},
				}
				return &b, nil
			},
			wantCode: 200,
		},
		{
			name:     "admin: no id",
			url:      "/admin",
			wantCode: 200,
			wantData: []string{
				"Create Book",
				"Set Admin Password",
			},
			unwantedData: []string{
				"Delete Book",
				"Update Book",
			},
		},
		{
			name: "admin: db error",
			url:  "/admin?book-id=BAD",
			readBook: func(id string) (*book.Book, error) {
				return nil, fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "admin: update book",
			url:  "/admin?book-id=5618941",
			readBook: func(id string) (*book.Book, error) {
				if id != "5618941" {
					return nil, fmt.Errorf("unwanted id: %v", id)
				}
				b := book.Book{
					Header:      book.Header{ID: "5618941"},
					Description: "info397",
				}
				return &b, nil
			},
			wantCode: 200,
			wantData: []string{
				"Delete Book",
				"Update Book",
				"Set Admin Password",
				"info397",
				"&lt;=&gt;",
			},
			unwantedData: []string{
				"Create Book",
				"<=>",
			},
		},
		{
			name: "book: db error",
			url:  "/book",
			readBook: func(id string) (*book.Book, error) {
				return nil, fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "book: happy path",
			url:  "/book?id=id7",
			readBook: func(id string) (*book.Book, error) {
				if id != "id7" {
					t.Errorf("unwanted id: %q", id)
				}
				b := book.Book{
					Header: book.Header{
						ID:    "id7",
						Title: "title8",
					},
					EAN_ISBN13: "weird_isbn",
				}
				return &b, nil
			},
			wantCode: 200,
			wantData: []string{"id7", "title8", "weird_isbn"},
		},
		{
			name:     "list: bad page",
			url:      "/list?page=last",
			wantCode: 400,
		},
		{
			name:     "list: bad filter",
			url:      "/list?q=(_invalid!!)",
			wantCode: 400,
		},
		{
			name:     "list: db error form",
			url:      "/list",
			wantCode: 500,
			readBookHeaders: func(f book.Filter, limit, offset int) ([]book.Header, error) {
				return nil, fmt.Errorf("db error")
			},
		},
		{
			name:     "list: empty form",
			url:      "/list",
			wantCode: 200,
			maxRows:  5,
			readBookHeaders: func(f book.Filter, limit, offset int) ([]book.Header, error) {
				headers := []book.Header{
					{Title: "hello"},
				}
				return headers, nil
			},
			wantData:     []string{"hello"},
			unwantedData: []string{`value="Load More books"`},
		},
		{
			name:     "list: page 3",
			url:      "/list?page=3&q=many+items&s=stuff",
			wantCode: 200,
			maxRows:  2,
			readBookHeaders: func(f book.Filter, limit, offset int) ([]book.Header, error) {
				wantFilter := book.Filter{HeaderParts: []string{"many", "items"}, Subject: "stuff"}
				switch {
				case !reflect.DeepEqual(wantFilter, f):
					return nil, fmt.Errorf("filters not equal: \n wanted: %v \n got:    %v", wantFilter, f)
				case limit < 2:
					return nil, fmt.Errorf("limit should be at least maxRows: %v", limit)
				case offset != 4:
					return nil, fmt.Errorf("unwanted offset: %v", offset)
				}
				headers := []book.Header{
					{Title: "Memo"},
					{Author: "Poe"},
					{ID: "MASTER_ID"}, // should be excluded because MaxRows is 2
				}
				return headers, nil
			},
			wantData: []string{
				"Memo",
				"Poe",
				`name="page" value="4"`,       // next page
				`name="q" value="many items"`, // preserve query when loading next page
				`value="Load More books"`,
			},
			unwantedData: []string{"MASTER_ID"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", test.url, nil)
			s := Server{
				Config: Config{MaxRows: test.maxRows},
				db: mockDatabase{
					readBookFunc:         test.readBook,
					readBookSubjectsFunc: test.readBookSubjects,
					readBookHeadersFunc:  test.readBookHeaders,
				},
			}
			h := s.mux()
			h.ServeHTTP(w, r)
			t.Run(test.name, func(t *testing.T) {
				switch {
				case test.wantCode != w.Code:
					t.Errorf("codes not equal: wanted %v, got %v", test.wantCode, w.Code)
				case w.Code == 200:
					got := w.Body.String()
					for _, want := range test.wantData {
						if !strings.Contains(got, want) {
							t.Errorf("wanted %q in body, got: \n %v", want, got)
						}
					}
					for _, exclude := range test.unwantedData {
						if strings.Contains(got, exclude) {
							t.Errorf("unwanted %q in body, got: \n %v", exclude, got)
						}
					}
				}
			})
		})
	}
}

func TestBookFrom(t *testing.T) {
	dateP := time.Date(2001, 6, 9, 0, 0, 0, 0, time.UTC)
	dateA := time.Date(2012, 12, 31, 0, 0, 0, 0, time.UTC)
	textPD := "2001-06-09"
	textAD := "2012-12-31"
	tests := []struct {
		name   string
		form   map[string]string
		want   *book.Book
		wantOk bool
	}{
		{"no title", map[string]string{}, nil, false},
		{"no author", map[string]string{"title": "a"}, nil, false},
		{"no subject", map[string]string{"title": "a", "author": "b"}, nil, false},
		{"no added Date", map[string]string{"title": "a", "author": "b", "subject": "c"}, nil, false},
		{"bad parse", map[string]string{"title": "a", "author": "b", "subject": "c", "added-date": textAD, "pages": "eight"}, nil, false},
		{"bad pages", map[string]string{"title": "a", "author": "b", "subject": "c", "added-date": textAD, "pages": "-1"}, nil, false},
		{
			name:   "long id (300 chars)",
			form:   map[string]string{"id": "012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789"},
			wantOk: false,
		},
		{
			name:   "minimal",
			form:   map[string]string{"title": "a", "author": "b", "subject": "c", "added-date": textAD, "pages": "8"},
			want:   &book.Book{Header: book.Header{Title: "a", Author: "b", Subject: "c"}, AddedDate: dateA, Pages: 8},
			wantOk: true,
		},
		{
			name: "all fields",
			form: map[string]string{
				"title":           "a",
				"author":          "b",
				"subject":         "c",
				"added-date":      textAD,
				"pages":           "8",
				"id":              "d",
				"description":     "e",
				"dewey-dec-class": "f",
				"publisher":       "g",
				"publish-date":    textPD,
				"ean-isbn-13":     "h",
				"upc-isbn-10":     "i",
			},
			want: &book.Book{
				Header: book.Header{
					ID:      "d",
					Title:   "a",
					Author:  "b",
					Subject: "c",
				},
				Description:   "e",
				DeweyDecClass: "f",
				Publisher:     "g",
				PublishDate:   dateP,
				AddedDate:     dateA,
				Pages:         8,
				EAN_ISBN13:    "h",
				UPC_ISBN10:    "i",
			},
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := multipartFormHelper(t, test.form)
			got, err := bookFrom(w, r)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case *test.want != *got:
				t.Errorf("not equal: \n wanted: %+v \n got:    %+v", *test.want, *got)
			}
		})
	}
}

func multipartFormHelper(t *testing.T, form map[string]string) *http.Request {
	t.Helper()
	var sb strings.Builder
	mpw := multipart.NewWriter(&sb)
	mpw.Close()
	r := httptest.NewRequest("POST", "/", strings.NewReader(sb.String()))
	r.Form = make(url.Values, len(form))
	for k, v := range form {
		r.Form.Set(k, v)
	}
	r.Header.Set("Content-Type", mpw.FormDataContentType())
	return r
}

func TestPostBook(t *testing.T) {
	tests := []struct {
		name         string
		form         map[string]string
		createBooks  func(books ...book.Book) ([]book.Book, error)
		wantCode     int
		wantLocation string
	}{
		{
			name: "bad book",
			form: map[string]string{
				"pages": "NaN",
			},
			wantCode: 400,
		},
		{
			name: "db error",
			form: map[string]string{
				"title":      "t",
				"author":     "a",
				"subject":    "s",
				"pages":      "1",
				"added-date": "2022-11-13",
			},
			createBooks: func(books ...book.Book) ([]book.Book, error) {
				return nil, fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "minimal happy path",
			form: map[string]string{
				"title":      "t",
				"author":     "a",
				"subject":    "s",
				"pages":      "1",
				"added-date": "2022-11-13",
			},
			createBooks: func(books ...book.Book) ([]book.Book, error) {
				want := book.Book{
					Header: book.Header{
						Title:   "t",
						Author:  "a",
						Subject: "s",
					},
					Pages:     1,
					AddedDate: time.Date(2022, 11, 13, 0, 0, 0, 0, time.UTC),
				}
				switch {
				case len(books) != 1:
					return nil, fmt.Errorf("wanted 1 book, got %v", len(books))
				case want != books[0]:
					return nil, fmt.Errorf("books not equal: \n wanted: %v \n got:    %v", want, books[0])
				}
				return []book.Book{{Header: book.Header{ID: "fg34"}}}, nil
			},
			wantCode:     303,
			wantLocation: "/book?id=fg34",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := Server{
				db: mockDatabase{
					createBooksFunc: test.createBooks,
				},
			}
			w := httptest.NewRecorder()
			r := multipartFormHelper(t, test.form)
			s.postBook(w, r)
			switch {
			case test.wantCode != w.Code:
				t.Errorf("codes not equal: wanted %v, got %v", test.wantCode, w.Code)
			case test.wantCode == 303:
				if want, got := test.wantLocation, w.Header().Get("Location"); want != got {
					t.Errorf("unwanted redirect location: \n wanted: %q \n got: %q", want, got)
				}
			}
		})
	}
}

func TestPutBook(t *testing.T) {
	tests := []struct {
		name          string
		formOverrides map[string]string
		updateBook    func(b book.Book, updateImage bool) error
		wantCode      int
		wantLocPrefix string
	}{
		{
			name: "bad book",
			formOverrides: map[string]string{
				"pages": "NaN",
			},
			wantCode: 400,
		},
		{
			name: "db error",
			updateBook: func(b book.Book, updateImage bool) error {
				return fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "minimal happy path",
			updateBook: func(b book.Book, updateImage bool) error {
				want := book.Book{
					Header: book.Header{
						ID:      "keep_me",
						Title:   "t",
						Author:  "a",
						Subject: "s",
					},
					Pages:     1,
					AddedDate: time.Date(2022, 11, 13, 0, 0, 0, 0, time.UTC),
				}
				switch {
				case want != b:
					return fmt.Errorf("books not equal: \n wanted: %v \n got:    %v", want, b)
				case updateImage:
					return fmt.Errorf("did not want to update image")
				}
				return nil
			},
			wantCode:      303,
			wantLocPrefix: "/book?id=keep_me",
		},
		{
			name: "update image",
			formOverrides: map[string]string{
				"update-image": "true",
			},
			updateBook: func(b book.Book, updateImage bool) error {
				switch {
				case !updateImage:
					return fmt.Errorf("did not want to update image")
				}
				return nil
			},
			wantCode:      303,
			wantLocPrefix: "/book?id=keep_me",
		},
		{
			name: "clear image",
			formOverrides: map[string]string{
				"update-image": "clear",
			},
			updateBook: func(b book.Book, updateImage bool) error {
				switch {
				case len(b.ImageBase64) != 0:
					return fmt.Errorf("wanted image to be zeroed")
				case !updateImage:
					return fmt.Errorf("did not want to update image")
				}
				return nil
			},
			wantCode:      303,
			wantLocPrefix: "/book?id=keep_me",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			form := map[string]string{
				"id":         "keep_me",
				"title":      "t",
				"author":     "a",
				"subject":    "s",
				"pages":      "1",
				"added-date": "2022-11-13",
			}
			for k, v := range test.formOverrides {
				form[k] = v
			}
			s := Server{
				db: mockDatabase{
					updateBookFunc: test.updateBook,
				},
			}
			w := httptest.NewRecorder()
			r := multipartFormHelper(t, form)
			s.putBook(w, r)
			switch {
			case test.wantCode != w.Code:
				t.Errorf("codes not equal: wanted %v, got %v", test.wantCode, w.Code)
			case test.wantCode == 303:
				if want, got := test.wantLocPrefix, w.Header().Get("Location"); !strings.HasPrefix(got, want) {
					t.Errorf("unwanted redirect location: \n wanted: %q \n got: %q", want, got)
				}
			}
		})
	}
}

func TestDeleteBook(t *testing.T) {
	tests := []struct {
		name         string
		deleteBook   func(id string) error
		wantCode     int
		wantData     []string
		unwantedData []string
	}{
		{
			name: "db error",
			deleteBook: func(id string) error {
				return fmt.Errorf("db error)")
			},
			wantCode: 500,
		},
		{
			name: "happy path",
			deleteBook: func(id string) error {
				if id != "x123" {
					return fmt.Errorf("unwanted id: %q", id)
				}
				return nil
			},
			wantCode: 303,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := Server{
				db: mockDatabase{
					deleteBookFunc: test.deleteBook,
				},
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest("post", "/book?id=x123", nil)
			s.deleteBook(w, r)
			switch {
			case test.wantCode != w.Code:
				t.Errorf("codes not equal: wanted %v, got %v", test.wantCode, w.Code)
			case test.wantCode == 303:
				if w.Header().Get("Location") != "/" {
					t.Errorf("wanted redirection to root")
				}
			}
		})
	}
}

func TestPutAdminPassword(t *testing.T) {
	tests := []struct {
		name                string
		form                url.Values
		hash                func(password []byte) (hashedPassword []byte, err error)
		updateAdminPassword func(hashedPassword string) error
		wantCode            int
	}{
		{
			name: "not equal",
			form: url.Values{
				"p1": {"bilbo123"},
				"p2": {"Bilbo123"},
			},
			wantCode: 400,
		},
		{
			name: "hash error",
			form: url.Values{
				"p1": {"bilbo123"},
				"p2": {"bilbo123"},
			},
			hash: func(password []byte) (hashedPassword []byte, err error) {
				return nil, fmt.Errorf("hash error")
			},
			wantCode: 500,
		},
		{
			name: "db error",
			form: url.Values{
				"p1": {"bilbo123"},
				"p2": {"bilbo123"},
			},
			hash: func(password []byte) (hashedPassword []byte, err error) {
				return []byte("X47"), nil
			},
			updateAdminPassword: func(hashedPassword string) error {
				return fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "happy path",
			form: url.Values{
				"p1": {"bilbo123"},
				"p2": {"bilbo123"},
			},
			hash: func(password []byte) (hashedPassword []byte, err error) {
				if string(password) != "bilbo123" {
					return nil, fmt.Errorf("unwanted password: %s", password)
				}
				return []byte("X47"), nil
			},
			updateAdminPassword: func(hashedPassword string) error {
				if hashedPassword != "X47" {
					return fmt.Errorf("unwanted hashedPassword: %v", hashedPassword)
				}
				return nil
			},
			wantCode: 303,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := Server{
				ph: mockPasswordHandler{
					hashFunc: test.hash,
				},
				db: mockDatabase{
					updateAdminPasswordFunc: test.updateAdminPassword,
				},
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest("post", "/book?id=x123", nil)
			r.Form = test.form
			s.putAdminPassword(w, r)
			switch {
			case test.wantCode != w.Code:
				t.Errorf("codes not equal: wanted %v, got %v", test.wantCode, w.Code)
			case test.wantCode == 303:
				if w.Header().Get("Location") != "/" {
					t.Errorf("wanted redirection to root")
				}
			}
		})
	}
}

func TestWithAdminPassword(t *testing.T) {
	tests := []struct {
		name              string
		form              url.Values
		readAdminPassword func() (hashedPassword []byte, err error)
		isCorrectPassword func(hashedPassword, password []byte) (ok bool, err error)
		wantCode          int
		wantData          string
	}{
		{
			name: "read error",
			readAdminPassword: func() (hashedPassword []byte, err error) {
				return nil, fmt.Errorf("read error")
			},
			wantCode: 500,
			wantData: "read error",
		},
		{
			name: "is correct error",
			readAdminPassword: func() (hashedPassword []byte, err error) {
				return nil, nil
			},
			isCorrectPassword: func(hashedPassword, password []byte) (ok bool, err error) {
				return false, fmt.Errorf("is correct error")
			},
			wantCode: 500,
			wantData: "is correct error",
		},
		{
			name: "incorrect",
			readAdminPassword: func() (hashedPassword []byte, err error) {
				return nil, nil
			},
			isCorrectPassword: func(hashedPassword, password []byte) (ok bool, err error) {
				return false, nil
			},
			wantCode: 401,
			wantData: "Unauthorized",
		},
		{
			name: "happy path",
			form: url.Values{
				"p": {"top_Secret-007"},
			},
			readAdminPassword: func() (hashedPassword []byte, err error) {
				return []byte("HAsh#"), nil
			},
			isCorrectPassword: func(hashedPassword, password []byte) (ok bool, err error) {
				switch {
				case string(hashedPassword) != "HAsh#":
					t.Errorf("unwanted hashed password: %s", hashedPassword)
				case string(password) != "top_Secret-007":
					t.Errorf("unwanted password: %s", password)
				}
				return true, nil
			},
			wantCode: 200,
			wantData: "validated",
		},
	}
	h1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("validated"))
	})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := http.Request{
				Form: test.form,
			}
			s := Server{
				db: mockDatabase{
					readAdminPasswordFunc: test.readAdminPassword,
				},
				ph: mockPasswordHandler{
					isCorrectPasswordFunc: test.isCorrectPassword,
				},
			}
			h2 := s.withAdminPassword(h1)
			h2.ServeHTTP(w, &r)
			if test.wantCode != w.Code {
				t.Errorf("codes not equal: wanted %v, got %v", test.wantCode, w.Code)
			}
			if want, got := test.wantData, w.Body.String(); !strings.Contains(got, want) {
				t.Errorf("wanted %q in body, got: \n %v", want, got)
			}
		})
	}
}

func TestSetupAdminPassword(t *testing.T) {
	tests := []struct {
		name                string
		adminPassword       string
		hash                func(password []byte) (hashedPassword []byte, err error)
		updateAdminPassword func(hashedPassword string) error
		wantOk              bool
	}{
		{
			name:          "tiny",
			adminPassword: "tiny",
		},
		{
			name:          "hash error",
			adminPassword: "bilbo+3Xr",
			hash: func(password []byte) (hashedPassword []byte, err error) {
				return nil, fmt.Errorf("hash error")
			},
		},
		{
			name:          "db error",
			adminPassword: "bilbo+3Xr",
			hash: func(password []byte) (hashedPassword []byte, err error) {
				return []byte("hash48"), nil
			},
			updateAdminPassword: func(hashedPassword string) error {
				return fmt.Errorf("db error")
			},
		},
		{
			name:          "happy path",
			adminPassword: "bilbo+3Xr",
			hash: func(password []byte) (hashedPassword []byte, err error) {
				if string(password) != "bilbo+3Xr" {
					return nil, fmt.Errorf("unwanted password: %q", password)
				}
				return []byte("hash48"), nil
			},
			updateAdminPassword: func(hashedPassword string) error {
				if hashedPassword != "hash48" {
					return fmt.Errorf("unwanted hashedPassword: %q", hashedPassword)
				}
				return nil
			},
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ph := mockPasswordHandler{
				hashFunc: test.hash,
			}
			db := mockDatabase{
				updateAdminPasswordFunc: test.updateAdminPassword,
			}
			cfg := Config{
				AdminPassword: test.adminPassword,
			}
			err := cfg.setup(db, ph, nil)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			}
		})
	}
}

func TestSetupBackfillCSV(t *testing.T) {
	tests := []struct {
		name   string
		db     Database
		wantOk bool
	}{
		{
			name: "db error",
			db: mockDatabase{
				createBooksFunc: func(books ...book.Book) ([]book.Book, error) {
					return nil, fmt.Errorf("db error")
				},
			},
		},
		{
			name: "csv db", // should not support createBooks
			db:   &csv.Database{},
		},
		{
			name: "happy path",
			db: mockDatabase{
				createBooksFunc: func(books ...book.Book) ([]book.Book, error) {
					if len(books) != 0 {
						return nil, fmt.Errorf("the embedded csv database should be empty when testing: got %v books", len(books))
					}
					return books, nil
				},
			},
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{
				BackfillCSV: true,
			}
			err := cfg.setup(test.db, nil, nil)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			}
		})
	}
}

func TestSetupDumpCSV(t *testing.T) {
	tests := []struct {
		name            string
		readBookHeaders func(f book.Filter, limit, offset int) ([]book.Header, error)
		readBook        func(id string) (*book.Book, error)
		wantOk          bool
		wantOut         string
	}{
		{
			name: "readBookHeaders error",
			readBookHeaders: func(f book.Filter, limit, offset int) ([]book.Header, error) {
				return nil, fmt.Errorf("readBookHeaders error")
			},
		},
		{
			name: "readBook error",
			readBookHeaders: func(f book.Filter, limit, offset int) ([]book.Header, error) {
				return []book.Header{{ID: "1"}}, nil
			},
			readBook: func(id string) (*book.Book, error) {
				return nil, fmt.Errorf("readBook error")
			},
		},
		{
			name: "happy path",
			readBookHeaders: func(f book.Filter, limit, offset int) ([]book.Header, error) {
				var headers []book.Header
				switch {
				case len(f.HeaderParts) != 0, len(f.Subject) != 0:
					return nil, fmt.Errorf("wanted no filter, got %v", f)
				case limit == 3 && offset == 0:
					headers = append(headers,
						book.Header{ID: "bk1"},
						book.Header{ID: "bk22"},
						book.Header{ID: "bk3"})
				case limit == 3 && offset == 2:
					headers = append(headers, book.Header{ID: "bk3"})
				default:
					return nil, fmt.Errorf("unwanted limit/offset: %v/%v", limit, offset)
				}
				return headers, nil
			},
			readBook: func(id string) (*book.Book, error) {
				b := book.Book{
					Header: book.Header{
						ID: id,
					},
					Description: id + "_description",
				}
				return &b, nil
			},
			wantOk: true,
			wantOut: `id,title,author,description,subject,dewey-dec-class,pages,publisher,publish-date,added-date,ean-isbn13,upc-isbn10,image-base64
bk1,,,bk1_description,,,0,,01/01/0001,01/01/0001,,,
bk22,,,bk22_description,,,0,,01/01/0001,01/01/0001,,,
bk3,,,bk3_description,,,0,,01/01/0001,01/01/0001,,,
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := mockDatabase{
				readBookHeadersFunc: test.readBookHeaders,
				readBookFunc:        test.readBook,
			}
			cfg := Config{
				DumpCSV: true,
				MaxRows: 2,
			}
			var sb strings.Builder
			err := cfg.setup(db, nil, &sb)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case test.wantOut != sb.String():
				t.Errorf("dumped csv not equal: \n wanted: %v \n got:    %v", test.wantOut, sb.String())
			}
		})
	}
}

func TestParseFormValue(t *testing.T) {
	tests := []struct {
		name      string
		form      url.Values
		key       string
		wantValue string
		maxLength int
		want      bool
		wantCode  int
	}{
		{"not set", url.Values{}, "k", "", 10, true, 200},
		{"happy path", url.Values{"a": {"b"}}, "a", "b", 10, true, 200},
		{"multiform", url.Values{"a": {"1"}, "b": {"2"}, "c": {"3"}}, "b", "2", 5, true, 200},
		{"too long", url.Values{"a": {"bamboozling"}}, "a", "", 10, false, 413},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			r.Form = test.form
			var dest string
			switch {
			case test.want != ParseFormValue(w, r, test.key, &dest, test.maxLength):
				t.Errorf("unwanted return value")
			case test.want && dest != test.wantValue:
				t.Errorf("value not set to %q: got %q", test.wantValue, dest)
			case test.wantCode != w.Code:
				t.Errorf("codes not equal: wanted %v, got, %v", test.wantCode, w.Code)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		p      string
		wantOk bool
	}{
		{"short", false},
		{"s3cr3t!%", true},
		{"Hello, 世界", false},
		{"￿1234567", false},
		{validPasswordRunes, true},
	}
	for i, test := range tests {
		err := validatePassword(test.p)
		gotOk := err == nil
		if test.wantOk != gotOk {
			t.Errorf("test %v: wanted valid: %v for %q", i, test.wantOk, test.p)
		}
	}
}

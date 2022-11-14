package server

import (
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
			mockReadBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
				return nil, nil
			},
			mockReadBookFunc: func(id string) (*book.Book, error) {
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
		{"list", "GET", "/", 200},
		{"book", "GET", "/book", 200},
		{"admin", "GET", "/admin", 200},
		{"robots.txt", "GET", "/robots.txt", 200},
		{"not found", "GET", "/bad.html", 404},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, test.url, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if want, got := test.wantCode, w.Code; want != got {
				t.Errorf("wanted %v, got %v", want, got)
			}
		})
	}
}

func TestWithCacheControl(t *testing.T) {
	msg := "OK_1549"
	h1 := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(msg))
	}
	h2 := withCacheControl(http.HandlerFunc(h1), time.Minute)
	r := httptest.NewRequest("", "/", nil)
	w := httptest.NewRecorder()
	h2.ServeHTTP(w, r)
	switch {
	case w.Body.String() != msg:
		t.Errorf("inner handler not run: wanted body to be %q, got %q", w.Body.String(), msg)
	default:
		want := "max-age=60"
		got := w.Result().Header.Get("Cache-Control")
		if want != got {
			t.Errorf("missing max-age Cache-Control header: got: %q", got)
		}
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

func TestResponseContains(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantBodyPart string
		db           Database
	}{
		{"MissingKeyZero", "/admin", `name="title" value="" required`, nil},
		{"TitleContainsQuote", "/admin?id=wow", `name="title" value="&#34;Wow,&#34; A Memoir" required`, mockDatabase{
			mockReadBookFunc: func(id string) (*book.Book, error) {
				b := book.Book{
					Header: book.Header{
						Title: `"Wow," A Memoir`,
					},
				}
				return &b, nil
			},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := Server{
				db: test.db,
			}
			h := s.mux()
			r := httptest.NewRequest("GET", test.url, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if want, got := 200, w.Code; want != got {
				t.Fatalf("codes: wanted %v, got %v", want, got)
			}
			if want, got := test.wantBodyPart, w.Body.String(); !strings.Contains(got, want) {
				t.Errorf("response body did not contain %q: \n %s", want, got)
			}
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
			r := multipartFormHelper(t, test.form)
			got, err := bookFrom(r)
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

func TestGetBookHeaders(t *testing.T) {
	tests := []struct {
		name         string
		form         url.Values
		s            Server
		wantCode     int
		wantData     []string
		unwantedData []string
	}{
		{
			name: "bad page",
			form: url.Values{
				"page": {"last"},
			},
			wantCode: 400,
		},
		{
			name: "bad filter",
			form: url.Values{
				"q": {"~!@#$%^&*(){}"},
			},
			wantCode: 400,
		},
		{
			name:     "db error form",
			wantCode: 500,
			s: Server{
				db: mockDatabase{
					mockReadBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						return nil, fmt.Errorf("db error")
					},
				},
			},
		},
		{
			name:     "empty form",
			wantCode: 200,
			s: Server{
				Config: Config{
					MaxRows: 5,
				},
				db: mockDatabase{
					mockReadBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						headers := []book.Header{
							{Title: "hello"},
						}
						return headers, nil
					},
				},
			},
			wantData:     []string{"hello"},
			unwantedData: []string{`value="Load More books"`},
		},
		{
			name: "page 23",
			form: url.Values{
				"page": {"3"},
				"q":    {"many items"},
			},
			wantCode: 200,
			s: Server{
				Config: Config{
					MaxRows: 2,
				},
				db: mockDatabase{
					mockReadBookHeadersFunc: func(f book.Filter, limit, offset int) ([]book.Header, error) {
						wantFilter := book.Filter{"many", "items"}
						switch {
						case !reflect.DeepEqual(wantFilter, f):
							return nil, fmt.Errorf("filters not equal: \n wanted: %v \n got:    %v", wantFilter, f)
						case limit < 2:
							return nil, fmt.Errorf("limit should be at least maxRows: %v", limit)
						case offset != 6:
							return nil, fmt.Errorf("unwanted offset: %v", offset)
						}
						headers := []book.Header{
							{Title: "Memo"},
							{Author: "Poe"},
							{ID: "MASTER_ID"}, // should be excluded because MaxRows is 2
						}
						return headers, nil
					},
				},
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
		w := httptest.NewRecorder()
		r := http.Request{
			Form: test.form,
		}
		test.s.getBookHeaders(w, &r)
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
	}
}

func TestGetBook(t *testing.T) {
	tests := []struct {
		name     string
		form     url.Values
		readBook func(id string) (*book.Book, error)
		wantCode int
		wantData []string
	}{
		{
			name: "db error",
			readBook: func(id string) (*book.Book, error) {
				return nil, fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "happy path",
			form: url.Values{
				"id": {"id7"},
			},
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
	}
	for _, test := range tests {
		w := httptest.NewRecorder()
		r := http.Request{
			Form: test.form,
		}
		s := Server{
			db: mockDatabase{
				mockReadBookFunc: test.readBook,
			},
		}
		s.getBook(w, &r)
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
			}
		})
	}
}

func TestGetAdmin(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		readBook     func(id string) (*book.Book, error)
		wantCode     int
		wantData     []string
		unwantedData []string
	}{
		{
			name:     "no id",
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
			name: "db error",
			url:  "/admin?id=BAD",
			readBook: func(id string) (*book.Book, error) {
				return nil, fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "update book",
			url:  "/admin?id=5618941",
			readBook: func(id string) (*book.Book, error) {
				if id != "5618941" {
					return nil, fmt.Errorf("unwanted id: %v", id)
				}
				b := book.Book{
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
			},
			unwantedData: []string{
				"Create Book",
			},
		},
	}
	for _, test := range tests {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("get", test.url, nil)
		s := Server{
			db: mockDatabase{
				mockReadBookFunc: test.readBook,
			},
		}
		s.getAdmin(w, r)
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
	}
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
					mockCreateBooksFunc: test.createBooks,
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
		updateBook    func(b book.Book, newID string, updateImage bool) error
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
			updateBook: func(b book.Book, newID string, updateImage bool) error {
				return fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "minimal happy path",
			updateBook: func(b book.Book, newID string, updateImage bool) error {
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
				case want != b:
					return fmt.Errorf("books not equal: \n wanted: %v \n got:    %v", want, b)
				case updateImage:
					return fmt.Errorf("did not want to update image")
				}
				return nil
			},
			wantCode:      303,
			wantLocPrefix: "/book?id=",
		},
		{
			name: "update image",
			formOverrides: map[string]string{
				"update-image": "true",
			},
			updateBook: func(b book.Book, newID string, updateImage bool) error {
				switch {
				case !updateImage:
					return fmt.Errorf("did not want to update image")
				}
				return nil
			},
			wantCode:      303,
			wantLocPrefix: "/book?id=",
		},
		{
			name: "clear image",
			formOverrides: map[string]string{
				"update-image": "clear",
			},
			updateBook: func(b book.Book, newID string, updateImage bool) error {
				switch {
				case len(b.ImageBase64) != 0:
					return fmt.Errorf("wanted image to be zeroed")
				case !updateImage:
					return fmt.Errorf("did not want to update image")
				}
				return nil
			},
			wantCode:      303,
			wantLocPrefix: "/book?id=",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			form := map[string]string{
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
					mockUpdateBookFunc: test.updateBook,
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
					mockDeleteBookFunc: test.deleteBook,
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
				"p1": {"bilbo"},
				"p2": {"Bilbo"},
			},
			wantCode: 400,
		},
		{
			name: "hash error",
			form: url.Values{
				"p1": {"bilbo"},
				"p2": {"bilbo"},
			},
			hash: func(password []byte) (hashedPassword []byte, err error) {
				return nil, fmt.Errorf("hash error")
			},
			wantCode: 500,
		},
		{
			name: "db error",
			form: url.Values{
				"p1": {"bilbo"},
				"p2": {"bilbo"},
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
				"p1": {"bilbo"},
				"p2": {"bilbo"},
			},
			hash: func(password []byte) (hashedPassword []byte, err error) {
				if string(password) != "bilbo" {
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
					mockHashFunc: test.hash,
				},
				db: mockDatabase{
					mockUpdateAdminPasswordFunc: test.updateAdminPassword,
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
					mockReadAdminPasswordFunc: test.readAdminPassword,
				},
				ph: mockPasswordHandler{
					mockIsCorrectPasswordFunc: test.isCorrectPassword,
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
		hash                func(password []byte) (hashedPassword []byte, err error)
		updateAdminPassword func(hashedPassword string) error
		wantOk              bool
	}{
		{
			name: "hash error",
			hash: func(password []byte) (hashedPassword []byte, err error) {
				return nil, fmt.Errorf("hash error")
			},
		},
		{
			name: "db error",
			hash: func(password []byte) (hashedPassword []byte, err error) {
				return []byte("hash48"), nil
			},
			updateAdminPassword: func(hashedPassword string) error {
				return fmt.Errorf("db error")
			},
		},
		{
			name: "happy path",
			hash: func(password []byte) (hashedPassword []byte, err error) {
				if string(password) != "leap17" {
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
				mockHashFunc: test.hash,
			}
			db := mockDatabase{
				mockUpdateAdminPasswordFunc: test.updateAdminPassword,
			}
			cfg := Config{
				AdminPassword: "leap17",
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

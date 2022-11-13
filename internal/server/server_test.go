package server

import (
	"compress/gzip"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
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
				MaxRows: 42,
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

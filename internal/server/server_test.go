package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
)

func TestMux(t *testing.T) {
	cfg := Config{
		DatabaseURL: "csv://",
	}
	s, err := cfg.NewServer(io.Discard)
	if err != nil {
		t.Fatal(err)
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
		{"book", "GET", "/book", 500}, // Missing id
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

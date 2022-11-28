package server

import (
	"net/http/httptest"
	"strings"
	"testing"
	"text/template"

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
				t.Errorf("configs not equal: \n wanted: %v \n got:    %v", got.Config, test.cfg)
			case got.db == nil:
				t.Errorf("database not set")
			case got.ph == nil:
				t.Errorf("password handler not set")
			case got.out != &sb:
				t.Errorf("output writers not equal: \n wanted: %v \n got:    %v", got.out, &sb)
			}
		})
	}
}

func TestEmbeddedCSVDatabase(t *testing.T) {
	db, err := embeddedCSVDatabase()
	switch {
	case err != nil:
		t.Errorf("unwanted error: %v", err)
	case len(db.Books) != 0:
		t.Errorf("wanted no books in saved library, got %v [Disable this test when backfilling books]", len(db.Books))
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
			var sb strings.Builder
			s.out = &sb
			r := httptest.NewRequest(test.method, test.url, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if want, got := test.wantCode, w.Code; want != got {
				t.Errorf("wanted %v, got %v", want, got)
			}
			if sb.Len() != 0 {
				t.Errorf("unwanted log: %q", sb.String())
			}
		})
	}
}

func TestServeTemplate(t *testing.T) {
	t.Run("template error", func(t *testing.T) {
		tmpl = new(template.Template)
		var sb strings.Builder
		s := Server{
			out: &sb,
		}
		w := httptest.NewRecorder()
		s.serveTemplate(w, "other", nil)
		if sb.Len() == 0 {
			t.Errorf("wanted error logged when template is empty")
		}
	})
}

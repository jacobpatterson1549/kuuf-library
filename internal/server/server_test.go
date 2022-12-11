package server

import (
	"context"
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
			ctx := context.Background()
			got, err := test.cfg.NewServer(ctx, &sb)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case got.cfg != test.cfg:
				t.Errorf("configs not equal: \n wanted: %v \n got:    %v", test.cfg, got.cfg)
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
	if err != nil {
		t.Fatalf("unwanted error: %v", err)
	}
	ctx := context.Background()
	var filter book.Filter
	limit := 1
	offset := 0
	headers, err := db.ReadBookHeaders(ctx, filter, limit, offset)
	switch {
	case err != nil:
		t.Errorf("unwanted error: %v", err)
	case len(headers) != 0:
		t.Errorf("wanted no books in saved library, got at least %v", len(headers))
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
		tmpl: parseTemplate(),
	}
	lim := &countRateLimiter{max: 1}
	h := s.mux(lim)
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
		tmpl := new(template.Template)
		var sb strings.Builder
		s := Server{
			tmpl: tmpl,
			out:  &sb,
		}
		w := httptest.NewRecorder()
		s.serveTemplate(w, "other", nil)
		if sb.Len() == 0 {
			t.Errorf("wanted error logged when template is empty")
		}
	})
}

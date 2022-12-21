package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"text/template"
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
			case got.tmpl == nil:
				t.Errorf("template not set")
			case got.staticFS != staticFS:
				t.Errorf("staticFS not set")
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

func TestRunInvalidServer(t *testing.T) {
	tests := []struct {
		name     string
		s        Server
		wantLogs []string
	}{
		{
			name: "setup failure",
			s: Server{
				cfg: Config{
					AdminPassword: "Backfill-M3",
				},
			},
		},
		{
			name: "no setup, bad port",
			s: Server{
				cfg: Config{
					Port: "@_bad:P0rt!",
				},
				db: mockDatabase{},
			},
			wantLogs: []string{
				"localhost:@_bad:P0rt!",
				"mockDatabase",
			},
		},
		// {name: "valid server should fail test"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sb strings.Builder
			test.s.out = &sb
			ctx := context.Background()
			ctx, cancelFunc := context.WithTimeout(ctx, time.Second)
			defer cancelFunc()
			go func() {
				if err := test.s.Run(ctx); err == nil {
					t.Errorf("wanted error running server")
				}
				cancelFunc()
			}()
			select {
			case <-ctx.Done():
				// NOOP
			case <-time.After(1 * time.Second):
				t.Fatalf("invalid server did not stop")
			}
			gotLog := sb.String()
			for _, wantLog := range test.wantLogs {
				if !strings.Contains(gotLog, wantLog) {
					t.Errorf("wanted log to contain %q:\n got %q", wantLog, gotLog)
				}
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
	if headers, err := db.ReadBookHeaders(ctx, filter, limit, offset); err != nil || len(headers) != 0 {
		t.Errorf("wanted no headers and no error, got: %v, %v", headers, err)
	}
	if subjects, err := db.ReadBookSubjects(ctx, 0, 0); err != nil || len(subjects) != 0 {
		t.Errorf("wanted no subjects and no error, got: %v, %v", subjects, err)
	}
	if _, err := db.ReadBook(ctx, "unknown-id"); err == nil {
		t.Errorf("wanted error reading book with unknown id")
	}
	if _, ok := db.(AllBooksDatabase); !ok {
		t.Fatalf("source is not an allBookIterator")
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
		tmpl:     parseTemplate(staticFS),
		staticFS: staticFS, // used by robots.txt
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

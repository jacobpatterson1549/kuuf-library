package server

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"golang.org/x/time/rate"
)

func TestQueryTimeout(t *testing.T) {
	cfg := Config{
		DBTimeoutSec: 105,
	}
	want := 105 * time.Second
	got := cfg.queryTimeout()
	if want != got {
		t.Errorf("not equal: \n wanted: %v \n got:    %v", want, got)
	}
}

func TestPostRateLimiter(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want *rate.Limiter
	}{
		{
			name: "empty",
			want: rate.NewLimiter(rate.Inf, 0),
		},
		{
			name: "ones",
			cfg: Config{
				PostLimitSec: 1,
				PostMaxBurst: 1,
			},
			want: rate.NewLimiter(1, 1),
		},
		{
			name: "3 requests allowed every 2 seconds",
			cfg: Config{
				PostLimitSec: 2,
				PostMaxBurst: 3,
			},
			want: rate.NewLimiter(0.5, 3),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if want, got := test.want, test.cfg.postRateLimiter(); !reflect.DeepEqual(want, got) {
				t.Errorf("not equal: \n wanted: %v \n got:    %v", want, got)
			}
		})
	}
}

func TestSetupInitAdminPassword(t *testing.T) {
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
		name       string
		libraryCSV string
		db         database
		wantOk     bool
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
			name:       "invalid csv db",
			libraryCSV: "INVALID,CSV",
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
			libraryCSV = test.libraryCSV
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
				case len(f.HeaderPart) != 0, len(f.Subject) != 0:
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

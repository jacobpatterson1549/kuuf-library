package server

import (
	"context"
	"fmt"
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

func TestGetRequest(t *testing.T) {
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
			name:     "MissingKeyZero",
			url:      "/admin",
			wantData: []string{`name="title" value="" required`},
			wantCode: 200,
		},
		{
			name:    "with space",
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
			name:     "TitleContainsQuote",
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
			url:  "/admin?book-id=BAD",
			readBook: func(id string) (*book.Book, error) {
				return nil, fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "update book",
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
			name:     "long id",
			url:      "/book?id=long+abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890",
			wantCode: 413,
		},
		{
			name: "db error",
			url:  "/book",
			readBook: func(id string) (*book.Book, error) {
				return nil, fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "happy path",
			url:  "/book?id=id7",
			readBook: func(id string) (*book.Book, error) {
				if id != "id7" {
					return nil, fmt.Errorf("unwanted id: %q", id)
				}
				b := book.Book{
					Header: book.Header{
						ID:      "id7",
						Title:   "title8",
						Author:  "a",
						Subject: "s",
					},
					DeweyDecClass: "ddc",
					Pages:         18,
					Publisher:     "pub",
					PublishDate:   time.Date(2022, 11, 25, 0, 0, 0, 0, time.UTC),
					AddedDate:     time.Date(2022, 11, 25, 0, 0, 0, 0, time.UTC),
					EanIsbn13:     "weird_isbn",
					UpcIsbn10:     "isbn10",
					ImageBase64:   "invalid_file",
				}
				return &b, nil
			},
			wantCode: 200,
			wantData: []string{"id7", "title8", "weird_isbn"},
		},
		{
			name:     "long filter",
			url:      "/list?q=TOO_LONG_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890",
			wantCode: 413,
		},
		{
			name:     "long subject",
			url:      "/list?s=TOO_LONG_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890",
			wantCode: 413,
		},
		{
			name:     "bad page",
			url:      "/list?page=last",
			wantCode: 400,
		},
		{
			name:     "long page",
			url:      "/list?page=1234567890123456789012345678901234567890",
			wantCode: 413,
		},
		{
			name:     "db error form",
			url:      "/list",
			wantCode: 500,
			readBookHeaders: func(f book.Filter, limit, offset int) ([]book.Header, error) {
				return nil, fmt.Errorf("db error")
			},
		},
		{
			name:     "empty form",
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
			name:     "page 3",
			url:      "/list?page=3&q=many+items&s=stuff",
			wantCode: 200,
			maxRows:  2,
			readBookHeaders: func(f book.Filter, limit, offset int) ([]book.Header, error) {
				wantFilter := book.Filter{HeaderPart: "many items", Subject: "stuff"}
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
		t.Run(test.name+" "+test.url, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", test.url, nil)
			var sb strings.Builder
			s := Server{
				cfg: Config{
					MaxRows: test.maxRows,
				},
				db: mockDatabase{
					readBookFunc:         test.readBook,
					readBookSubjectsFunc: test.readBookSubjects,
					readBookHeadersFunc:  test.readBookHeaders,
				},
				tmpl: parseTemplate(staticFS),
				out:  &sb,
			}
			var lim countRateLimiter
			h := s.mux(&lim)
			h.ServeHTTP(w, r)
			switch {
			case sb.Len() != 0:
				t.Errorf("unwanted log: %q", sb.String())
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

func TestPostRequest(t *testing.T) {
	tests := []struct {
		name                string
		url                 string
		form                map[string]string
		createBooks         func(books ...book.Book) ([]book.Book, error)
		updateBook          func(b book.Book, updateImage bool) error
		hash                func(password []byte) (hashedPassword []byte, err error)
		updateAdminPassword func(hashedPassword string) error
		deleteBook          func(id string) error
		wantCode            int
		wantLocation        string
	}{
		{
			name: "bad book",
			url:  "/book/create",
			form: map[string]string{
				"pages": "NaN",
			},
			wantCode: 400,
		},
		{
			name: "db error",
			url:  "/book/create",
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
			url:  "/book/create",
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
		{
			name: "bad book",
			url:  "/book/update",
			form: map[string]string{
				"id":         "keep_me",
				"title":      "t",
				"author":     "a",
				"subject":    "s",
				"pages":      "NaN",
				"added-date": "2022-11-13",
			},
			wantCode: 400,
		},
		{
			name: "db error",
			url:  "/book/update",
			form: map[string]string{
				"id":         "keep_me",
				"title":      "t",
				"author":     "a",
				"subject":    "s",
				"pages":      "1",
				"added-date": "2022-11-13",
			},
			updateBook: func(b book.Book, updateImage bool) error {
				return fmt.Errorf("db error")
			},
			wantCode: 500,
		},
		{
			name: "minimal happy path",
			url:  "/book/update",
			form: map[string]string{
				"id":         "keep_me",
				"title":      "t",
				"author":     "a",
				"subject":    "s",
				"pages":      "1",
				"added-date": "2022-11-13",
			},
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
			wantCode:     303,
			wantLocation: "/book?id=keep_me",
		},
		{
			name: "update image too long",
			url:  "/book/update",
			form: map[string]string{
				"id":           "keep_me",
				"title":        "t",
				"author":       "a",
				"subject":      "s",
				"pages":        "1",
				"added-date":   "2022-11-13",
				"update-image": "123456789+long",
			},
			wantCode: 413,
		},
		{
			name: "update image",
			url:  "/book/update",
			form: map[string]string{
				"id":           "keep_me",
				"title":        "t",
				"author":       "a",
				"subject":      "s",
				"pages":        "1",
				"added-date":   "2022-11-13",
				"update-image": "true",
			},
			updateBook: func(b book.Book, updateImage bool) error {
				switch {
				case !updateImage:
					return fmt.Errorf("did not want to update image")
				}
				return nil
			},
			wantCode:     303,
			wantLocation: "/book?id=keep_me",
		},
		{
			name: "clear image",
			url:  "/book/update",
			form: map[string]string{
				"id":           "keep_me",
				"title":        "t",
				"author":       "a",
				"subject":      "s",
				"pages":        "1",
				"added-date":   "2022-11-13",
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
			wantCode:     303,
			wantLocation: "/book?id=keep_me",
		},
		{
			name: "long id",
			url:  "/book/delete",
			form: map[string]string{
				"id": "long+abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890",
			},
			deleteBook: func(id string) error {
				return fmt.Errorf("db error)")
			},
			wantCode: 413,
		},
		{
			name: "db error",
			url:  "/book/delete",
			form: map[string]string{
				"id": "x123",
			},
			deleteBook: func(id string) error {
				return fmt.Errorf("db error)")
			},
			wantCode: 500,
		},
		{
			name: "happy path",
			url:  "/book/delete",
			form: map[string]string{
				"id": "x123",
			},
			deleteBook: func(id string) error {
				if id != "x123" {
					return fmt.Errorf("unwanted id: %q", id)
				}
				return nil
			},
			wantCode:     303,
			wantLocation: "/",
		},
		{
			name: "too long",
			url:  "/admin/update",
			form: map[string]string{
				"p1": "TOO_LONG_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890",
			},
			wantCode: 413,
		},
		{
			name: "not equal",
			url:  "/admin/update",
			form: map[string]string{
				"p1": "bilbo123",
				"p2": "Bilbo123",
			},
			wantCode: 400,
		},
		{
			name: "too short",
			url:  "/admin/update",
			form: map[string]string{
				"p1": "bilbo",
				"p2": "bilbo",
			},
			wantCode: 400,
		},
		{
			name: "hash error",
			url:  "/admin/update",
			form: map[string]string{
				"p1": "bilbo123",
				"p2": "bilbo123",
			},
			hash: func(password []byte) (hashedPassword []byte, err error) {
				return nil, fmt.Errorf("hash error")
			},
			wantCode: 500,
		},
		{
			name: "db error",
			url:  "/admin/update",
			form: map[string]string{
				"p1": "bilbo123",
				"p2": "bilbo123",
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
			url:  "/admin/update",
			form: map[string]string{
				"p1": "bilbo123",
				"p2": "bilbo123",
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
			wantCode:     303,
			wantLocation: "/",
		},
	}
	for _, test := range tests {
		t.Run(test.name+" "+test.url, func(t *testing.T) {
			test.form["p"] = "v4lid_P"
			s := Server{
				db: mockDatabase{
					createBooksFunc:         test.createBooks,
					updateBookFunc:          test.updateBook,
					deleteBookFunc:          test.deleteBook,
					updateAdminPasswordFunc: test.updateAdminPassword,
					readAdminPasswordFunc: func() (hashedPassword []byte, err error) {
						return []byte("H#shed+P"), nil
					},
				},
				ph: mockPasswordHandler{
					hashFunc: test.hash,
					isCorrectPasswordFunc: func(hashedPassword, password []byte) (ok bool, err error) {
						return string(hashedPassword) == "H#shed+P" && string(password) == "v4lid_P", nil
					},
				},
				pv: passwordValidatorConfig{
					minLength:  8,
					validRunes: "bilbo123",
				}.NewPasswordValidator(),
				tmpl: parseTemplate(staticFS),
			}
			w := httptest.NewRecorder()
			r := multipartFormHelper(t, test.url, test.form)
			lim := countRateLimiter{max: 1}
			h := s.mux(&lim)
			h.ServeHTTP(w, r)
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
			name: "password too long",
			form: url.Values{
				"p": {"TOO_LONG_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"},
			},
			wantCode: 413,
		},
		{
			name: "password not set in database",
			readAdminPassword: func() (hashedPassword []byte, err error) {
				return []byte{}, nil
			},
			wantCode: 503,
		},
		{
			name: "is correct error",
			readAdminPassword: func() (hashedPassword []byte, err error) {
				return []byte("HAsh#"), nil
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
				return []byte("HAsh#"), nil
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
			db := mockDatabase{
				readAdminPasswordFunc: test.readAdminPassword,
			}
			ph := mockPasswordHandler{
				isCorrectPasswordFunc: test.isCorrectPassword,
			}
			pv := passwordValidatorConfig{
				minLength:  8,
				validRunes: validPasswordRunes,
			}.NewPasswordValidator()
			s := Server{
				db: db,
				ph: ph,
				pv: pv,
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
				EanIsbn13:     "h",
				UpcIsbn10:     "i",
			},
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := multipartFormHelper(t, "/", test.form)
			ctx := context.Background()
			got, err := bookFrom(ctx, w, r)
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

func multipartFormHelper(t *testing.T, target string, form map[string]string) *http.Request {
	t.Helper()
	var sb strings.Builder
	mpw := multipart.NewWriter(&sb)
	mpw.Close()
	r := httptest.NewRequest("POST", target, strings.NewReader(sb.String()))
	r.Form = make(url.Values, len(form))
	for k, v := range form {
		r.Form.Set(k, v)
	}
	r.Header.Set("Content-Type", mpw.FormDataContentType())
	return r
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
			case test.want != parseFormValue(w, r, test.key, &dest, test.maxLength):
				t.Errorf("unwanted return value")
			case test.want && dest != test.wantValue:
				t.Errorf("value not set to %q: got %q", test.wantValue, dest)
			case test.wantCode != w.Code:
				t.Errorf("codes not equal: wanted %v, got, %v", test.wantCode, w.Code)
			}
		})
	}
}

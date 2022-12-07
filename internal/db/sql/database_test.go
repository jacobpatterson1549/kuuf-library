package sql

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jacobpatterson1549/kuuf-library/internal/book"
	"github.com/jacobpatterson1549/kuuf-library/internal/db/sql/mock"
)

var testDriver mock.Driver
var testDriveInfo = driverInfo{
	ILike: "mock_ILIKE",
}

const testDriverName = "mock driver"

func init() {
	sql.Register(testDriverName, &testDriver)
	drivers[testDriverName] = testDriveInfo
}

func databaseHelper(t *testing.T, conn mock.Conn) *Database {
	t.Helper()
	testDriver.OpenFunc = func(name string) (mock.Conn, error) {
		return conn, nil
	}
	db, err := sql.Open(testDriverName, "")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	d := Database{
		db:           db,
		QueryTimeout: time.Hour,
	}
	return &d
}

func TestNewDatabase(t *testing.T) {
	tests := []struct {
		name             string
		driverName       string
		url              string
		queryTimeout     time.Duration
		openFunc         func(name string) (mock.Conn, error)
		wantOk           bool
		wantDriver       driverInfo
		wantQueryTimeout time.Duration
	}{
		{
			name:       "unknown driverName",
			driverName: "unknown",
		},
		{
			name:       "open db error",
			driverName: testDriverName,
			openFunc: func(name string) (mock.Conn, error) {
				return mock.Conn{}, fmt.Errorf("open error (on tx)")
			},
			queryTimeout: time.Hour,
		},
		{
			name:       "happy path (create user)",
			driverName: testDriverName,
			openFunc: func(name string) (mock.Conn, error) {
				return mock.NewTransactionConn(*mock.NewAnyQuery(0), *mock.NewAnyQuery(0), *mock.NewAnyQuery(1)), nil
			},
			queryTimeout:     time.Hour,
			wantOk:           true,
			wantDriver:       testDriveInfo,
			wantQueryTimeout: time.Hour,
		},
		{
			name:       "happy path",
			driverName: testDriverName,
			openFunc: func(name string) (mock.Conn, error) {
				return mock.NewTransactionConn(*mock.NewAnyQuery(0), *mock.NewAnyQuery(0), *mock.NewAnyQuery(0)), nil
			},
			queryTimeout:     37 * time.Hour,
			wantOk:           true,
			wantDriver:       testDriveInfo,
			wantQueryTimeout: 37 * time.Hour,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testDriver.OpenFunc = test.openFunc
			got, err := NewDatabase(test.driverName, test.url, test.queryTimeout)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case test.wantDriver != got.driver:
				t.Errorf("drivers not equal: \n wanted: %v \n got:    %v", test.wantDriver, got.driver)
			case test.wantQueryTimeout != got.QueryTimeout:
				t.Errorf("query timeouts not equal: \n wanted: %v \n got:    %v", test.wantQueryTimeout, got.QueryTimeout)
			}
		})
	}
}

func TestExecTxError(t *testing.T) {
	tests := []struct {
		name    string
		conn    mock.Conn
		queries []query
	}{
		{
			name: "exec error",
			conn: mock.Conn{
				PrepareFunc: func(query string) (driver.Stmt, error) {
					return mock.Stmt{
						NumInputFunc: func() int {
							return -1
						},
						ExecFunc: func(args []driver.Value) (driver.Result, error) {
							return nil, fmt.Errorf("exec error")
						},
						CloseFunc: func() error {
							return nil
						},
					}, nil
				},
				BeginFunc: func() (driver.Tx, error) {
					return mock.Tx{
						RollbackFunc: func() error {
							return nil
						},
						CommitFunc: func() error {
							return nil
						},
					}, nil
				},
			},
			queries: []query{{cmd: "DROP unknown"}},
		},
		{
			name: "rows affected error error",
			conn: mock.Conn{
				PrepareFunc: func(query string) (driver.Stmt, error) {
					return mock.Stmt{
						NumInputFunc: func() int {
							return -1
						},
						ExecFunc: func(args []driver.Value) (driver.Result, error) {
							return mock.Result{
								RowsAffectedFunc: func() (int64, error) {
									return 0, fmt.Errorf("mock error")
								},
							}, nil
						},
						CloseFunc: func() error {
							return nil
						},
					}, nil
				},
				BeginFunc: func() (driver.Tx, error) {
					return mock.Tx{
						RollbackFunc: func() error {
							return fmt.Errorf("rollback error")
						},
						CommitFunc: func() error {
							return nil
						},
					}, nil
				},
			},
			queries: []query{{cmd: "DROP unknown"}},
		},
		{
			name: "exec/rollback error",
			conn: mock.Conn{
				PrepareFunc: func(query string) (driver.Stmt, error) {
					return mock.Stmt{
						NumInputFunc: func() int {
							return -1
						},
						ExecFunc: func(args []driver.Value) (driver.Result, error) {
							return nil, fmt.Errorf("exec error")
						},
						CloseFunc: func() error {
							return nil
						},
					}, nil
				},
				BeginFunc: func() (driver.Tx, error) {
					return mock.Tx{
						RollbackFunc: func() error {
							return fmt.Errorf("rollback error")
						},
						CommitFunc: func() error {
							return nil
						},
					}, nil
				},
			},
			queries: []query{{cmd: "DROP unknown"}},
		},
		{
			name: "commit error",
			conn: mock.Conn{
				BeginFunc: func() (driver.Tx, error) {
					return mock.Tx{
						CommitFunc: func() error {
							return fmt.Errorf("commit error")
						},
					}, nil
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := databaseHelper(t, test.conn)
			if err := d.execTx(test.queries...); err == nil {
				t.Errorf("wanted error")
			}
		})
	}
}

func TestQueryScanError(t *testing.T) {
	conn := mock.Conn{
		PrepareFunc: func(query string) (driver.Stmt, error) {
			return mock.Stmt{
				NumInputFunc: func() int {
					return -1
				},
				QueryFunc: func(args []driver.Value) (driver.Rows, error) {
					return mock.Rows{
						ColumnsFunc: func() []string {
							return []string{"column1"}
						},
						NextFunc: func(dest []driver.Value) error {
							return nil
						},
						CloseFunc: func() error {
							return nil
						},
					}, nil
				},
				CloseFunc: func() error {
					return nil
				},
			}, nil
		},
	}
	d := databaseHelper(t, conn)
	dest := func() []interface{} {
		return nil // expects destination for column1
	}
	q := query{
		cmd:  "any",
		args: []interface{}{},
	}
	if err := d.query(q, dest); err == nil {
		t.Errorf("wanted error when query expects 1 argument")
	}
}

func TestCreateBooks(t *testing.T) {
	d1 := time.Date(2003, 6, 9, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2004, 12, 31, 0, 0, 0, 0, time.UTC)
	wantInsert := "INSERT INTO books (id, title, author, subject, description, dewey_dec_class, pages, publisher, publish_date, added_date, ean_isbn13, upc_isbn10, image_base64) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)"
	tests := []struct {
		name   string
		conn   mock.Conn
		books  []book.Book
		wantOk bool
	}{
		{
			name: "db error",
			conn: mock.Conn{
				BeginFunc: func() (driver.Tx, error) {
					return nil, fmt.Errorf("db error")
				},
			},
		},
		{
			name:   "happy path: no books",
			conn:   mock.NewTransactionConn(),
			wantOk: true,
		},
		{
			name: "happy path: one book",
			conn: mock.NewTransactionConn(
				mock.Query{
					Name:         wantInsert,
					Args:         []interface{}{mock.AnyArg, "t1", "a1", "s1", "d1", "ddc1", 2, "p1", d1, d2, "ean", "upc", "?"},
					RowsAffected: 1,
				},
			),
			books: []book.Book{
				{
					Header:      book.Header{ID: "", Title: "t1", Author: "a1", Subject: "s1"},
					Description: "d1", DeweyDecClass: "ddc1", Pages: 2, Publisher: "p1",
					PublishDate: d1, AddedDate: d2, EanIsbn13: "ean", UpcIsbn10: "upc",
					ImageBase64: "?",
				},
			},
			wantOk: true,
		},
		{
			name: "happy path: two books",
			conn: mock.NewTransactionConn(
				mock.Query{
					Name:         wantInsert,
					Args:         []interface{}{mock.AnyArg, "", "", "", "", "", 14, "", time.Time{}, time.Time{}, "", "", ""},
					RowsAffected: 1,
				},
				mock.Query{
					Name:         wantInsert,
					Args:         []interface{}{mock.AnyArg, "Title2", "", "", "", "", 0, "", time.Time{}, time.Time{}, "", "", ""},
					RowsAffected: 1,
				},
			),
			books: []book.Book{
				{Pages: 14},
				{Header: book.Header{Title: "Title2"}},
			},
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := make([]book.Book, len(test.books))
			copy(want, test.books)
			d := databaseHelper(t, test.conn)
			got, err := d.CreateBooks(test.books...)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			default:
				for i, b := range got {
					if len(b.ID) == 0 {
						t.Errorf("id of book %v not set", i)
					}
					got[i].ID = ""
				}
				if !reflect.DeepEqual(want, got) {
					t.Errorf("book contents not equal [excluding ids]: \n wanted: %v \n got:    %v", want, got)
				}
			}
		})
	}
}

func TestReadBookSubjects(t *testing.T) {
	wantQuery := "SELECT subject, COUNT(*) FROM books GROUP BY subject ORDER BY subject ASC LIMIT $1 OFFSET $2"
	tests := []struct {
		name   string
		limit  int
		offset int
		conn   mock.Conn
		wantOk bool
		want   []book.Subject
	}{
		{
			name: "db error",
			conn: mock.Conn{
				PrepareFunc: func(query string) (driver.Stmt, error) {
					return nil, fmt.Errorf("db error")
				},
			},
		},
		{
			name:  "more than limit",
			limit: 0,
			conn: mock.NewQueryConn(
				mock.Query{
					Name: wantQuery,
					Args: []interface{}{0, 0},
				},
				[][]interface{}{
					{"elephants", 8},
				}),
		},
		{
			name:   "happy path",
			limit:  2,
			offset: 3,
			conn: mock.NewQueryConn(
				mock.Query{
					Name: wantQuery,
					Args: []interface{}{2, 3},
				},
				[][]interface{}{
					{"elephants", 8},
					{"lizards", 7},
				}),
			wantOk: true,
			want: []book.Subject{
				{Name: "elephants", Count: 8},
				{Name: "lizards", Count: 7},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := databaseHelper(t, test.conn)
			d.driver.ILike = "LK"
			got, err := d.ReadBookSubjects(test.limit, test.offset)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("books not equal: \n wanted: %q \n got:    %q", test.want, got)
			}
		})
	}
}

func TestReadBookHeaders(t *testing.T) {
	wantQuery := "SELECT id, title, author, subject FROM books WHERE ($1 OR subject = $2) AND ($3 OR title LK $4 OR author LK $4 OR subject LK $4) ORDER BY subject ASC, Title ASC LIMIT $5 OFFSET $6"
	tests := []struct {
		name   string
		filter book.Filter
		offset int
		limit  int
		conn   mock.Conn
		wantOk bool
		want   []book.Header
	}{
		{
			name: "db error",
			conn: mock.Conn{
				PrepareFunc: func(query string) (driver.Stmt, error) {
					return nil, fmt.Errorf("db error")
				},
			},
		},
		{
			name:  "more than limit",
			limit: 1,
			conn: mock.NewQueryConn(
				mock.Query{
					Name: wantQuery,
					Args: []interface{}{true, "", true, "%%", 1, 0},
				},
				[][]interface{}{
					{"x1", "cats", "a3", "SBJ"},
					{"a0", "cats", "b2", "SBJ"},
				}),
			want: []book.Header{},
		},
		{
			name: "no filter",
			conn: mock.NewQueryConn(
				mock.Query{
					Name: wantQuery,
					Args: []interface{}{true, "", true, "%%", 0, 0},
				},
				[][]interface{}{}),
			wantOk: true,
			want:   []book.Header{},
		},
		{
			name:   "happy path with filter",
			filter: book.Filter{Subject: "SBJ", HeaderPart: "cat"},
			limit:  5,
			offset: 100,
			conn: mock.NewQueryConn(
				mock.Query{
					Name: wantQuery,
					Args: []interface{}{false, "SBJ", false, "%cat%", 5, 100},
				},
				[][]interface{}{
					{"x1", "cats", "a3", "SBJ"},
					{"a0", "cats", "b2", "SBJ"},
				}),
			wantOk: true,
			want: []book.Header{
				{ID: "x1", Title: "cats", Author: "a3", Subject: "SBJ"},
				{ID: "a0", Title: "cats", Author: "b2", Subject: "SBJ"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := databaseHelper(t, test.conn)
			d.driver.ILike = "LK"
			got, err := d.ReadBookHeaders(test.filter, test.limit, test.offset)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("books not equal: \n wanted: %q \n got:    %q", test.want, got)
			}
		})
	}
}

func TestReadBook(t *testing.T) {
	d0 := time.Date(1999, 12, 6, 0, 0, 0, 0, time.UTC)
	d1 := time.Date(2022, 12, 6, 0, 0, 0, 0, time.UTC)
	wantSelect := "SELECT id, title, author, subject, description, dewey_dec_class, pages, publisher, publish_date, added_date, ean_isbn13, upc_isbn10, image_base64 FROM books WHERE id = $1"
	tests := []struct {
		name   string
		bookID string
		conn   mock.Conn
		wantOk bool
		want   *book.Book
	}{
		{
			name: "db error",
			conn: mock.Conn{
				PrepareFunc: func(query string) (driver.Stmt, error) {
					return nil, fmt.Errorf("db error")
				},
			},
		},
		{
			name:   "no result",
			bookID: "b52",
			conn: mock.NewQueryConn(
				mock.Query{
					Name: wantSelect,
					Args: []interface{}{"b52"},
				},
				[][]interface{}{},
			),
		},
		{
			name:   "happy path",
			bookID: "b52",
			conn: mock.NewQueryConn(
				mock.Query{
					Name: wantSelect,
					Args: []interface{}{"b52"},
				},
				[][]interface{}{
					{"id0", "t2", "a3", "s4", "d5", "ddc6", 7, "p8", d0, d1, "EAN", "UPC", "IMG"},
				},
			),
			wantOk: true,
			want: &book.Book{
				Header:      book.Header{ID: "id0", Title: "t2", Author: "a3", Subject: "s4"},
				Description: "d5", DeweyDecClass: "ddc6", Pages: 7, Publisher: "p8",
				PublishDate: d0, AddedDate: d1, EanIsbn13: "EAN", UpcIsbn10: "UPC", ImageBase64: "IMG",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := databaseHelper(t, test.conn)
			got, err := d.ReadBook(test.bookID)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			case !reflect.DeepEqual(test.want, got):
				t.Errorf("books not equal: \n wanted: %q \n got:    %q", test.want, got)
			}
		})
	}
}

func TestUpdateBook(t *testing.T) {
	d1 := time.Date(2001, 6, 9, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2012, 12, 31, 0, 0, 0, 0, time.UTC)
	const (
		wantUpdateBasic = "UPDATE books SET title = $1, author = $2, subject = $3, description = $4, dewey_dec_class = $5, pages = $6, publisher = $7, publish_date = $8, added_date = $9, ean_isbn13 = $10, upc_isbn10 = $11 WHERE id = $12"
		wantUpdateImage = "UPDATE books SET title = $1, author = $2, subject = $3, description = $4, dewey_dec_class = $5, pages = $6, publisher = $7, publish_date = $8, added_date = $9, ean_isbn13 = $10, upc_isbn10 = $11, image_base64 = $12 WHERE id = $13"
	)
	tests := []struct {
		name        string
		b           book.Book
		updateImage bool
		conn        mock.Conn
		wantOk      bool
	}{
		{
			name: "db error",
			conn: mock.Conn{
				BeginFunc: func() (driver.Tx, error) {
					return nil, fmt.Errorf("db error")
				},
			},
		},
		{
			name: "happy path",
			b: book.Book{
				Header:      book.Header{ID: "b81", Title: "t1", Author: "a1", Subject: "s1"},
				Description: "d1", DeweyDecClass: "ddc1", Pages: 9, Publisher: "p1",
				PublishDate: d1, AddedDate: d2, EanIsbn13: "ean", UpcIsbn10: "upc",
			},
			conn: mock.NewTransactionConn(
				mock.Query{
					Name:         wantUpdateBasic,
					Args:         []interface{}{"t1", "a1", "s1", "d1", "ddc1", int64(9), "p1", d1, d2, "ean", "upc", "b81"},
					RowsAffected: 1,
				},
			),
			wantOk: true,
		},
		{
			name: "happy path - updateImage",
			b: book.Book{
				Header:      book.Header{ID: "b82", Title: "t2", Author: "a2", Subject: "s2"},
				Description: "d2", DeweyDecClass: "ddc2", Pages: 4, Publisher: "p2",
				PublishDate: d2, AddedDate: d1, EanIsbn13: "ean", UpcIsbn10: "upc",
				ImageBase64: "333",
			},
			updateImage: true,
			conn: mock.NewTransactionConn(
				mock.Query{
					Name:         wantUpdateImage,
					Args:         []interface{}{"t2", "a2", "s2", "d2", "ddc2", int64(4), "p2", d2, d1, "ean", "upc", "333", "b82"},
					RowsAffected: 1,
				},
			),
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := databaseHelper(t, test.conn)
			err := d.UpdateBook(test.b, test.updateImage)
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

func TestDeleteBook(t *testing.T) {
	tests := []struct {
		name   string
		bookID string
		conn   mock.Conn
		wantOk bool
	}{
		{
			name: "db error",
			conn: mock.Conn{
				BeginFunc: func() (driver.Tx, error) {
					return nil, fmt.Errorf("db error")
				},
			},
		},
		{
			name:   "happy path",
			bookID: "113=zoom",
			conn: mock.NewTransactionConn(
				mock.Query{
					Name:         "DELETE FROM books WHERE id = $1",
					Args:         []interface{}{"113=zoom"},
					RowsAffected: 1,
				},
			),
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := databaseHelper(t, test.conn)
			err := d.DeleteBook(test.bookID)
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

func TestReadAdminPassword(t *testing.T) {
	tests := []struct {
		name         string
		conn         mock.Conn
		wantOk       bool
		wantPassword string
	}{
		{
			name: "db error",
			conn: mock.Conn{
				PrepareFunc: func(query string) (driver.Stmt, error) {
					return nil, fmt.Errorf("db error")
				},
			},
		},
		{
			name: "two passwords",
			conn: mock.NewQueryConn(
				mock.Query{
					Name: "SELECT password FROM users WHERE username = $1",
					Args: []interface{}{"admin"},
				},
				[][]interface{}{
					{"p3pp3r$"},
					{"eXtra!"},
				},
			),
		},
		{
			name: "happy path",
			conn: mock.NewQueryConn(
				mock.Query{
					Name: "SELECT password FROM users WHERE username = $1",
					Args: []interface{}{"admin"},
				},
				[][]interface{}{
					{"p3pp3r$"},
				},
			),
			wantOk:       true,
			wantPassword: "p3pp3r$",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := databaseHelper(t, test.conn)
			hashedPassword, err := d.ReadAdminPassword()
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			default:
				if want, got := test.wantPassword, string(hashedPassword); want != got {
					t.Errorf("hashed passwords not equal: \n wanted: %q \n got:    %q", want, got)
				}
			}
		})
	}
}

func TestUpdateAdminPassword(t *testing.T) {
	tests := []struct {
		name           string
		hashedPassword string
		conn           mock.Conn
		wantOk         bool
	}{
		{
			name: "db error",
			conn: mock.Conn{
				BeginFunc: func() (driver.Tx, error) {
					return nil, fmt.Errorf("db error")
				},
			},
		},
		{
			name:           "bad update count",
			hashedPassword: "H4#h",
			conn: mock.NewTransactionConn(
				mock.Query{
					Name: "UPDATE users SET password = $1 WHERE username = $2",
					Args: []interface{}{"H4#h", "admin"},
				},
			),
		},
		{
			name:           "happy path",
			hashedPassword: "H4#h",
			conn: mock.NewTransactionConn(
				mock.Query{
					Name:         "UPDATE users SET password = $1 WHERE username = $2",
					Args:         []interface{}{"H4#h", "admin"},
					RowsAffected: 1,
				},
			),
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := databaseHelper(t, test.conn)
			err := d.UpdateAdminPassword(test.hashedPassword)
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

package mock

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"
)

var testDriver Driver

const testDriverName = "mocks"

func init() {
	sql.Register(testDriverName, &testDriver)
}

func TestNewQuery(t *testing.T) {
	got := Query{
		Name:         "abc",
		Args:         []interface{}{1, "Bear", 3.14},
		RowsAffected: 8,
	}
	want := Query{
		Name: "abc",
		Args: []interface{}{
			1,
			"Bear",
			3.14,
		},
		RowsAffected: 8,
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("queries not equal: \n wanted: %#v \n got:    %#v", want, got)
	}
}

func TestQueryCheckEquals(t *testing.T) {
	want := Query{
		Name:         "Hello, World!",
		Args:         []interface{}{1, "two", 3.14, AnyArg},
		RowsAffected: 55, // should not be checked
	}
	if err := want.checkEquals("Hello, World!", 1, "two", 3.14, 42); err != nil {
		t.Errorf("unwanted error: %v", err)
	}
}

func TestDriverValue(t *testing.T) {
	q := Query{
		Name:         "xyz",
		Args:         []interface{}{9, ":)", false},
		RowsAffected: 7,
	}
	want := Query{
		Name:         "xyz",
		Args:         []interface{}{int64(9), ":)", false},
		RowsAffected: 7,
	}
	got := q.driverValue()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("driver values not equal: \n wanted: %#v \n got:    %#v", want, got)
	}
	q.Args[2] = true
	if q.Args[2] == got.Args[2] {
		t.Errorf("driver value should not have a shallow copy of the arguments")
	}
}

func TestQueryConn(t *testing.T) {
	tests := []struct {
		name      string
		queryConn Conn
		wantQuery Query
		wantOk    bool
		want      [][]interface{}
		got       [][]interface{} // should be initialized with zero values of desired types
	}{
		{
			name:      "queries not equal",
			queryConn: NewQueryConn(Query{Name: "A"}, nil),
			wantQuery: Query{Name: "B"},
		},
		{
			name:      "args not equal",
			queryConn: NewQueryConn(Query{Name: "A", Args: []interface{}{"C", "D"}}, nil),
			wantQuery: Query{Name: "A", Args: []interface{}{"C", "E"}},
		},
		{
			name: "happy path",
			queryConn: NewQueryConn(
				Query{Name: "SELECT 'mock query', $1;", Args: []interface{}{42}},
				[][]interface{}{{"mock query", 42}},
			),
			wantQuery: Query{Name: "SELECT 'mock query', $1;", Args: []interface{}{42}},
			wantOk:    true,
			want:      [][]interface{}{{"mock query", 42}},
			got:       [][]interface{}{{"", 0}},
		},
		{
			name:      "happy path: no results",
			queryConn: NewQueryConn(Query{Name: "SELECT 1 WHERE 2 = 3;"}, [][]interface{}{}),
			wantQuery: Query{Name: "SELECT 1 WHERE 2 = 3;"},
			wantOk:    true,
			want:      [][]interface{}{},
			got:       [][]interface{}{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testDriver.OpenFunc = func(name string) (Conn, error) {
				return test.queryConn, nil
			}
			db, err := sql.Open(testDriverName, "")
			if err != nil {
				t.Fatalf("opening database: %v", err)
			}
			rows, err := db.Query(test.wantQuery.Name, test.wantQuery.Args...)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted query error")
				}
			case err != nil:
				t.Errorf("unwanted query error: %v", err)
			default:
				defer rows.Close()
				var index int
				for rows.Next() {
					dest := make([]interface{}, len(test.got[index]))
					for i := range dest {
						dest[i] = &test.got[index][i]
					}
					if err := rows.Scan(dest...); err != nil {
						if !test.wantOk {
							return // error desired
						}
						t.Errorf("unwanted scan error: %v", err)
					}
					index++
				}
				err := rows.Err()
				switch {
				case !test.wantOk:
					if err == nil {
						t.Errorf("wanted scan error")
					}
				case err != nil:
					t.Errorf("unwanted scan error: %v", err)
				case !reflect.DeepEqual(test.want, test.got):
					t.Errorf("results not equal: \n wanted: %v \n got:    %v", test.want, test.got)
				}
			}
		})
	}
	t.Run("beginTx not allowed", func(t *testing.T) {
		testDriver.OpenFunc = func(name string) (Conn, error) {
			return NewQueryConn(Query{}, nil), nil
		}
		db, err := sql.Open(testDriverName, "")
		if err != nil {
			t.Fatalf("opening database: %v", err)
		}
		if _, err := db.Begin(); err == nil {
			t.Errorf("wanted error")
		}
	})
}

func TestTransactionConn(t *testing.T) {
	tests := []struct {
		name         string
		txConn       Conn
		wantCommands []Query
		wantOk       bool
	}{
		{
			name:   "no commands",
			txConn: NewTransactionConn(),
			wantOk: true,
		},
		{
			name: "wrong query",
			txConn: NewTransactionConn(
				Query{Name: "c1"},
			),
			wantCommands: []Query{
				{Name: "c2"},
			},
		},
		{
			name: "bad rows affected",
			txConn: NewTransactionConn(
				Query{Name: "c1", RowsAffected: 3},
			),
			wantCommands: []Query{
				{Name: "c2", RowsAffected: 2},
			},
		},
		{
			name: "happy path",
			txConn: NewTransactionConn(
				Query{
					Name:         "c1",
					Args:         []interface{}{"uno", 0.1},
					RowsAffected: 1,
				},
				Query{
					Name:         "c2",
					Args:         []interface{}{"dos", 0.2},
					RowsAffected: 2,
				},
			),
			wantCommands: []Query{
				{
					Name:         "c1",
					Args:         []interface{}{"uno", 0.1},
					RowsAffected: 1,
				},
				{
					Name:         "c2",
					Args:         []interface{}{"dos", 0.2},
					RowsAffected: 2,
				},
			},
			wantOk: true,
		},
		{
			name: "any query",
			txConn: NewTransactionConn(
				*NewAnyQuery(-1),
			),
			wantCommands: []Query{
				{
					Name:         "c1",
					Args:         []interface{}{"uno", 0.1},
					RowsAffected: 1,
				},
			},
			wantOk: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testDriver.OpenFunc = func(name string) (Conn, error) {
				return test.txConn, nil
			}
			db, err := sql.Open(testDriverName, "")
			if err != nil {
				t.Fatalf("opening database: %v", err)
			}
			tx, err := db.Begin()
			if err == nil {
				for _, c := range test.wantCommands {
					var result driver.Result
					result, err = tx.Exec(c.Name, c.Args...)
					if err != nil {
						break
					}
					var gotRowsAffected int64
					gotRowsAffected, err = result.RowsAffected()
					if err != nil {
						break
					}
					if want, got := c.RowsAffected, gotRowsAffected; got != -1 && want != got {
						err = fmt.Errorf("rows affected not equal: wanted %v, got %v", want, got)
						break
					}
				}
				switch {
				case err != nil:
					err = tx.Rollback()
				default:
					err = tx.Commit()
				}
			}
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

func TestNotImplemented(t *testing.T) {
	tests := []struct {
		name    string
		errFunc func() error
	}{
		{
			name: "Result: LastInsertId",
			errFunc: func() error {
				var r Result
				_, err := r.LastInsertId()
				return err
			},
		},
		{
			name: "Connection: Close",
			errFunc: func() error {
				var conn Conn
				return conn.Close()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.errFunc(); err == nil {
				t.Error()
			}
		})
	}
}

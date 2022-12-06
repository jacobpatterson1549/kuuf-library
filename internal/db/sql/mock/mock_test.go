package mock

import (
	"database/sql"
	"reflect"
	"testing"
)

var testDriver Driver

const testDriverName = "mocks"

func init() {
	sql.Register(testDriverName, &testDriver)
}

func TestNewQuery(t *testing.T) {
	got := NewQuery("abc", 1, "Bear", 3.14)
	want := Query{
		Name: "abc",
		Args: []interface{}{
			1,
			"Bear",
			3.14,
		},
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("queries not equal: \n wanted: %v \n got:    %v", want, got)
	}
}

func TestQueryCheckEquals(t *testing.T) {
	want := NewQuery("Hello, World!", 1, "two", 3.14, AnyArg)
	if err := want.checkEquals("Hello, World!", 1, "two", 3.14, 42); err != nil {
		t.Errorf("unwanted error: %v", err)
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
			queryConn: NewQueryConn(NewQuery("A"), nil),
			wantQuery: NewQuery("B"),
		},
		{
			name:      "args not equal",
			queryConn: NewQueryConn(NewQuery("A", "C", "D"), nil),
			wantQuery: NewQuery("A", "C", "E"),
		},
		{
			name:      "happy path",
			queryConn: NewQueryConn(NewQuery("SELECT 'mock query', $1;", 42), [][]interface{}{{"mock query", 42}}),
			wantQuery: NewQuery("SELECT 'mock query', $1;", 42),
			wantOk:    true,
			want:      [][]interface{}{{"mock query", 42}},
			got:       [][]interface{}{{"", 0}},
		},
		{
			name:      "happy path: no results",
			queryConn: NewQueryConn(NewQuery("SELECT 1 WHERE 2 = 3;"), [][]interface{}{}),
			wantQuery: NewQuery("SELECT 1 WHERE 2 = 3;"),
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
				NewQuery("c1"),
			),
			wantCommands: []Query{
				NewQuery("c2"),
			},
		},
		{
			name: "happy path",
			txConn: NewTransactionConn(
				NewQuery("c1", 1, "uno", 0.1),
				NewQuery("c2", 2, "dos", 0.2),
			),
			wantCommands: []Query{
				NewQuery("c1", 1, "uno", 0.1),
				NewQuery("c2", 2, "dos", 0.2),
			},
			wantOk: true,
		},
		{
			name: "any query",
			txConn: NewTransactionConn(
				AnyQuery,
			),
			wantCommands: []Query{
				NewQuery("c1", 1, "uno", 0.1),
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
					if _, err = tx.Exec(c.Name, c.Args...); err != nil {
						break
					}
				}
				if err == nil {
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
				// TODO: check results
			}
		})
	}
}

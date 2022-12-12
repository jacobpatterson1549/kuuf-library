// Package mock provides a mock sql driver for tests.
// Register the driver with the database/sql package:
//
//	sql.Register("mock-driver-name", &mock.Driver{})
package mock

import (
	"database/sql/driver"
	"fmt"
	"io"
	"reflect"
)

type (
	// Driver implements the sql/driver.Conn interface.
	Driver struct {
		OpenFunc func(name string) (Conn, error)
	}
	// Query simplifies sending arguments/constraints to custom connections.
	Query struct {
		Name         string
		Args         []interface{}
		RowsAffected int64
	}
	// Conn implements the sql/driver.Conn interface.
	Conn struct {
		PrepareFunc func(query string) (driver.Stmt, error)
		BeginFunc   func() (driver.Tx, error)
	}
	// Stmt implements the sql/driver.Stmt interface.
	Stmt struct {
		CloseFunc    func() error
		NumInputFunc func() int
		ExecFunc     func(args []driver.Value) (driver.Result, error)
		QueryFunc    func(args []driver.Value) (driver.Rows, error)
	}
	// Tx implements the sql/driver/Tx interface.
	Tx struct {
		CommitFunc   func() error
		RollbackFunc func() error
	}
	// Result implements the sql/driver.Result interface.
	Result struct {
		RowsAffectedFunc func() (int64, error)
	}
	// Rows implements the sql/driver.Rows interface.
	Rows struct {
		ColumnsFunc func() []string
		CloseFunc   func() error
		NextFunc    func(dest []driver.Value) error
	}
)

var (
	// AnyArg represents an argument whose value might be dynamic (randomized).
	AnyArg       = &struct{ name string }{"any argument"}
	anyQueryName = "magic_value"
	anyQueryArgs = []interface{}{"should+match+any+query+1549"}
)

func (m *Driver) Open(name string) (driver.Conn, error) {
	return m.OpenFunc(name)
}

// NewAnyQuery creates a query that allows any name/args.
func NewAnyQuery(rowsAffected int64) *Query {
	q := Query{
		Name:         anyQueryName,
		Args:         anyQueryArgs,
		RowsAffected: rowsAffected,
	}
	return &q
}

func (q Query) isAny() bool {
	return q.Name == anyQueryName && reflect.DeepEqual(q.Args, anyQueryArgs)
}

func (q Query) checkEquals(query string, args ...driver.Value) error {
	if q.isAny() {
		return nil
	}
	if want, got := q.Name, query; want != got {
		return fmt.Errorf("queries not equal: \n wanted: %q \n got:    %q", want, got)
	}
	for i, wantArg := range q.Args {
		if wantArg != AnyArg {
			gotArg := args[i]
			if want, got := wantArg, gotArg; !reflect.DeepEqual(want, got) {
				return fmt.Errorf("index %v: args not equal: wanted: %#v got: %#v", i, want, got)
			}
		}
	}
	return nil
}

func (q Query) driverValue() Query {
	q2 := q
	args1 := q.Args
	q2.Args = make([]interface{}, len(args1))
	copy(q2.Args, args1)
	for i, a := range q2.Args {
		switch a := a.(type) {
		case int:
			q2.Args[i] = int64(a) // driver.Value casts all ints to int64
		}
	}
	return q2
}

func NewQueryConn(query Query, results [][]interface{}) Conn {
	want := query.driverValue()
	return Conn{
		PrepareFunc: func(query string) (driver.Stmt, error) {
			return Stmt{
				NumInputFunc: func() int {
					return len(want.Args)
				},
				CloseFunc: func() error {
					return nil
				},
				QueryFunc: func(args []driver.Value) (driver.Rows, error) {
					if err := want.checkEquals(query, args...); err != nil {
						return nil, err
					}
					var rowIndex int
					return Rows{
						ColumnsFunc: func() []string {
							if len(results) == 0 {
								return nil
							}
							return make([]string, len(results[0]))
						},
						CloseFunc: func() error {
							return nil
						},
						NextFunc: func(dest []driver.Value) error {
							if rowIndex >= len(results) {
								return io.EOF
							}
							row := results[rowIndex]
							rowIndex++
							for i, src := range row {
								dest[i] = src
							}
							return nil
						},
					}, nil
				},
			}, nil
		},
		BeginFunc: func() (driver.Tx, error) {
			return nil, fmt.Errorf("not implemented")
		},
	}
}

func NewTransactionConn(commands ...Query) Conn {
	var commandIndex int
	return Conn{
		BeginFunc: func() (driver.Tx, error) {
			return Tx{
				CommitFunc: func() error {
					return nil
				},
				RollbackFunc: func() error {
					return fmt.Errorf("unwanted rollback")
				},
			}, nil
		},
		PrepareFunc: func(query string) (driver.Stmt, error) {
			return Stmt{
				NumInputFunc: func() int {
					q := commands[commandIndex]
					if q.isAny() {
						return -1
					}
					return len(q.Args)
				},
				CloseFunc: func() error {
					return nil
				},
				ExecFunc: func(args []driver.Value) (driver.Result, error) {
					q := commands[commandIndex].driverValue()
					commandIndex++
					if err := q.checkEquals(query, args...); err != nil {
						return nil, err
					}
					return Result{
						RowsAffectedFunc: func() (int64, error) {
							return q.RowsAffected, nil
						},
					}, nil
				},
			}, nil
		},
	}
}

func (m Conn) Prepare(query string) (driver.Stmt, error) {
	return m.PrepareFunc(query)
}

func (m Conn) Close() error {
	return fmt.Errorf("not implemented")
}

func (m Conn) Begin() (driver.Tx, error) {
	return m.BeginFunc()
}

func (m Stmt) Close() error {
	return m.CloseFunc()
}

func (m Stmt) NumInput() int {
	return m.NumInputFunc()
}

func (m Stmt) Exec(args []driver.Value) (driver.Result, error) {
	return m.ExecFunc(args)
}

func (m Stmt) Query(args []driver.Value) (driver.Rows, error) {
	return m.QueryFunc(args)
}

func (m Tx) Commit() error {
	return m.CommitFunc()
}

func (m Tx) Rollback() error {
	return m.RollbackFunc()
}

func (m Result) LastInsertId() (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (m Result) RowsAffected() (int64, error) {
	return m.RowsAffectedFunc()
}

func (m Rows) Columns() []string {
	return m.ColumnsFunc()
}

func (m Rows) Close() error {
	return m.CloseFunc()
}

func (m Rows) Next(dest []driver.Value) error {
	return m.NextFunc(dest)
}

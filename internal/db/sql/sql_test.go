package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"

	"github.com/jacobpatterson1549/kuuf-library/internal/db/sql/mock"
)

var testDriver mock.Driver

const testDriverName = "mock driver"

func init() {
	sql.Register(testDriverName, &testDriver)
}

func dbHelper(t *testing.T, conn mock.Conn) *db {
	t.Helper()
	testDriver.OpenFunc = func(name string) (mock.Conn, error) {
		return conn, nil
	}
	sqlDB, err := sql.Open(testDriverName, "")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	d := db{
		db: sqlDB,
	}
	return &d
}

func TestExecOK(t *testing.T) {
	c1 := mock.Query{
		Name:         "DELETE FROM stuff WHERE old = true",
		RowsAffected: 9,
	}
	c2 := mock.Query{
		Name:         "UPDATE stuffLog SET recentlyDeleted = true WHERE userID = %1",
		Args:         []interface{}{"userIDx"},
		RowsAffected: 1,
	}
	conn := mock.NewTransactionConn(c1, c2)
	d := dbHelper(t, conn)
	cmds := []query{
		{cmd: c1.Name, args: c1.Args, wantedRowsAffected: []int64{11, 9, 10}}, // The command should affect between 9-11 rows
		{cmd: c2.Name, args: c2.Args, wantedRowsAffected: []int64{c2.RowsAffected}},
	}
	ctx := context.Background()
	if err := d.execTx(ctx, cmds...); err != nil {
		t.Errorf("unwanted error: %v", err)
	}
}

func TestExecTxError(t *testing.T) {
	tests := []struct {
		name    string
		conn    mock.Conn
		queries []query
	}{
		{
			name: "begin error",
			conn: mock.Conn{
				BeginFunc: func() (driver.Tx, error) {
					return nil, fmt.Errorf("begin error")
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
		{
			name: "bad rows affected",
			conn: mock.NewTransactionConn(
				mock.Query{
					Name:         "DELETE FROM users WHERE id = $1",
					Args:         []interface{}{6},
					RowsAffected: 66,
				},
			),
			queries: []query{
				{
					cmd:                "DELETE FROM users WHERE id = $1",
					args:               []interface{}{6},
					wantedRowsAffected: []int64{1},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := dbHelper(t, test.conn)
			ctx := context.Background()
			if err := d.execTx(ctx, test.queries...); err == nil {
				t.Errorf("wanted error")
			}
		})
	}
}

func TestQueryOK(t *testing.T) {
	q := query{
		cmd:  "SELECT fullName FROM users WHERE ID = $1",
		args: []interface{}{32},
	}
	conn :=
		mock.NewQueryConn(
			mock.Query{
				Name: q.cmd,
				Args: q.args,
			},
			[][]interface{}{
				{"Fred Flintstone"},
				{"Barney Rubble"},
			})
	d := dbHelper(t, conn)

	var i int
	got := make([]string, 2)
	dest := func() []interface{} {
		j := i
		i++
		return []interface{}{&got[j]}
	}
	ctx := context.Background()
	err := d.query(ctx, q, dest)
	want := []string{
		"Fred Flintstone",
		"Barney Rubble",
	}
	switch {
	case err != nil:
		t.Errorf("unwanted error: %v", err)
	case !reflect.DeepEqual(want, got):
		t.Errorf("results not equal: \n wanted: %q \n got:    %q", want, got)
	}
}

func TestQueryError(t *testing.T) {
	tests := []struct {
		name string
		conn mock.Conn
	}{
		{
			name: "query error",
			conn: mock.Conn{
				PrepareFunc: func(query string) (driver.Stmt, error) {
					return nil, fmt.Errorf("query error")
				},
			},
		},
		{
			name: "scan error",
			conn: mock.Conn{
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
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := dbHelper(t, test.conn)
			dest := func() []interface{} {
				return nil // expects destination for column1
			}
			q := query{
				cmd:  "any",
				args: []interface{}{},
			}
			ctx := context.Background()
			if err := d.query(ctx, q, dest); err == nil {
				t.Errorf("wanted error")
			}
		})
	}
}

func TestQueryRowQK(t *testing.T) {
	wantQuery := mock.Query{
		Name: "SELECT col1, col2, col3 FROM stuff WHERE id = $1",
		Args: []interface{}{"arg1"},
	}
	wantResult := []interface{}{"arg1", 1, true}
	conn := mock.NewQueryConn(wantQuery, [][]interface{}{wantResult})
	d := dbHelper(t, conn)
	q := query{
		cmd:  wantQuery.Name,
		args: wantQuery.Args,
	}
	var (
		gotCol1 string
		gotCol2 int
		gotCol3 bool
	)
	ctx := context.Background()
	err := d.queryRow(ctx, q, &gotCol1, &gotCol2, &gotCol3)
	gotResult := []interface{}{gotCol1, gotCol2, gotCol3}
	switch {
	case err != nil:
		t.Errorf("unwanted error: %v", err)
	case !reflect.DeepEqual(wantResult, gotResult):
		t.Errorf("results not equal: \n wanted: %q \n got:    %q", wantResult, gotResult)
	}
}

func TestQueryRowError(t *testing.T) {
	tests := []struct {
		name string
		conn mock.Conn
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
			name: "multiple rows",
			conn: mock.NewQueryConn(
				mock.Query{
					Name: "SELECT fullName FROM users WHERE ID = %1",
					Args: []interface{}{32},
				},
				[][]interface{}{
					{"Fred Flintstone"},
					{"Barney Rubble"},
				}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := dbHelper(t, test.conn)
			var fullName string
			q := query{
				cmd:  "SELECT fullName FROM users WHERE ID = %1",
				args: []interface{}{32},
			}
			ctx := context.Background()
			err := d.queryRow(ctx, q, &fullName)
			if err == nil {
				t.Errorf("wanted error")
			}
		})
	}
}

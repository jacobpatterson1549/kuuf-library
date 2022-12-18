package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
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
			d := dbHelper(t, test.conn)
			ctx := context.Background()
			if err := d.execTx(ctx, test.queries...); err == nil {
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
	d := dbHelper(t, conn)
	dest := func() []interface{} {
		return nil // expects destination for column1
	}
	q := query{
		cmd:  "any",
		args: []interface{}{},
	}
	ctx := context.Background()
	if err := d.query(ctx, q, dest); err == nil {
		t.Errorf("wanted error when query expects 1 argument")
	}
}

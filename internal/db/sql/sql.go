package sql

import (
	"context"
	"database/sql"
	"fmt"
)

type db struct {
	db *sql.DB
}

func (q query) execute(ctx context.Context, tx *sql.Tx) error {
	result, err := tx.ExecContext(ctx, q.cmd, q.args...)
	if err != nil {
		return fmt.Errorf("executing query: %w", err)
	}
	got, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if !q.allowsRowsAffected(got) {
		return fmt.Errorf("unwanted rows affected: %v", got)
	}
	return nil
}

func (q query) allowsRowsAffected(target int64) bool {
	for _, v := range q.wantedRowsAffected {
		if v == target {
			return true
		}
	}
	return false
}

func (d *db) execTx(ctx context.Context, queries ...query) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	for _, q := range queries {
		if err = q.execute(ctx, tx); err != nil {
			break
		}
	}
	if err != nil {
		if err2 := tx.Rollback(); err2 != nil {
			err = fmt.Errorf("rollback error: %v, root cause: %w", err, err2)
		}
		return fmt.Errorf("executing transaction queries: %w", err)
	}
	if err != tx.Commit() {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

func (d *db) query(ctx context.Context, q query, dest func() []interface{}) error {
	rows, err := d.db.QueryContext(ctx, q.cmd, q.args...)
	if err != nil {
		return fmt.Errorf("running query: %w", err)
	}
	defer rows.Close()
	for i := 0; rows.Next(); i++ {
		if err := rows.Scan(dest()...); err != nil {
			return fmt.Errorf("scanning row %v: %w", i, err)
		}
	}
	return nil
}

func (d *db) queryRow(ctx context.Context, q query, dest ...interface{}) error {
	n := 0
	destF := func() []interface{} {
		if n != 0 {
			return nil
		}
		n++
		return dest
	}
	err := d.query(ctx, q, destF)
	switch {
	case err != nil:
		return err
	case n != 1:
		return fmt.Errorf("wanted to get 1 row, got %v", n)
	}
	return nil
}

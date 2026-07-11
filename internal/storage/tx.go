package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// WithTx executes fn inside a database transaction. If fn returns an error,
// the transaction is rolled back. Otherwise, it is committed. WithTx handles
// begin/commit/rollback so callers don't have to.
func WithTx(ctx context.Context, db *DB, fn func(tx *sql.Tx) error) error {
	if db == nil || db.DB == nil {
		return fmt.Errorf("storage: nil database")
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: begin tx: %w", err)
	}

	// Rollback on panic.
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p) // re-throw
		}
	}()

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: commit tx: %w", err)
	}

	return nil
}

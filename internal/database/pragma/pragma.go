// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pragma

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/database/txn"
)

// Pragma is the name of a pragma.
type Pragma string

const (
	// ForeignKeysPragma is the name of the foreign keys pragma.
	ForeignKeysPragma Pragma = "foreign_keys"
)

// GetPragma returns whether the given pragma is enabled.
func GetPragma[T any](ctx context.Context, txn coredatabase.TxnRunner, pragam Pragma) (T, error) {
	var value T
	err := txn.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("PRAGMA %s", pragam)
		err := tx.QueryRowContext(ctx, query).Scan(&value)
		return errors.Trace(err)
	})
	if err != nil {
		return value, fmt.Errorf("failed to get %q pragma: %w", pragam, err)
	}
	return value, nil
}

var (
	// Reuse the txn runner for retries as they're consistent. We can't use
	// the database package directly, as that causes import cycle error.
	runner = txn.NewRetryingTxnRunner()
)

// SetPragma sets the given pragma to the given value.
func SetPragma[T any](ctx context.Context, db *sql.DB, pragma Pragma, value T) error {
	query := fmt.Sprintf("PRAGMA %s = %v", pragma, value)
	err := runner.Retry(ctx, func() error {
		_, err := db.ExecContext(ctx, query)
		return errors.Trace(err)
	})
	if err != nil {
		return fmt.Errorf("failed to set %q pragma: %w", pragma, err)
	}
	return nil
}

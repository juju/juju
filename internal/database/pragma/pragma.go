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

	// OptimizePragma is the name of the optimize pragma. Running it asks
	// SQLite to run ANALYZE against any table whose query planner statistics
	// in sqlite_stat1 have gone stale.
	OptimizePragma Pragma = "optimize"
)

// The constants below are the bits of the mask accepted by the optimize
// pragma. See https://sqlite.org/pragma.html#pragma_optimize
const (
	// optimizeDebug reports the ANALYZE statements that would have been run,
	// without running any of them.
	optimizeDebug = 0x00001

	// optimizeAnalyze runs ANALYZE on the tables that might benefit from it.
	// SQLite ignores the pragma entirely when this bit is not set, so it is
	// required by both of the masks below.
	optimizeAnalyze = 0x00002

	// optimizeAnalysisLimit applies a temporary analysis limit for the
	// duration of the pragma, which bounds how long the ANALYZE invocations
	// can run for.
	optimizeAnalysisLimit = 0x00010

	// optimizeAllTables considers every table, rather than only those that
	// the current connection has used statistics for. See Optimize for why
	// this is not optional for us.
	optimizeAllTables = 0x10000

	// optimizeMask is the mask used to optimize a database.
	optimizeMask = optimizeAnalyze | optimizeAnalysisLimit | optimizeAllTables

	// optimizeDryRunMask is optimizeMask, reporting the work rather than
	// doing it.
	optimizeDryRunMask = optimizeMask | optimizeDebug
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

// Optimize runs the optimize pragma against the database, refreshing the
// query planner statistics for any table that has grown or shrunk by more
// than 10-fold since it was last analysed. It is usually a no-op, and fast.
//
// Two Dqlite specific constraints shape how this is run, and both of them
// are load-bearing:
//
//   - It must not be run inside a transaction. The pragma performs its
//     ANALYZE work in sub-statements, which means SQLite can report the
//     statement as read-only even though it writes. Dqlite decides whether
//     to claim its single writer slot from exactly that flag, so a write
//     left open inside an explicit transaction trips an assertion in the
//     Dqlite leader, which aborts the process.
//
//   - The optimizeAllTables bit is not optional. Without it SQLite only
//     considers tables whose statistics the current connection has already
//     used, and only with it set is the statement always classified as a
//     writer, so that Dqlite serialises it against other writers as it
//     should. It also suits our connection pooling, where each pooled
//     connection is a distinct Dqlite leader connection with its own session
//     state, and so on its own only ever sees a fraction of the workload.
//
// Note that new statistics are only picked up by connections that load the
// schema afterwards. The first run creates the sqlite_stat1 table, which
// bumps the schema cookie and so refreshes every connection. Later runs
// update existing rows, and are picked up as connections are recycled.
func Optimize(ctx context.Context, db *sql.DB) error {
	query := fmt.Sprintf("PRAGMA %s(0x%x)", OptimizePragma, optimizeMask)
	err := runner.Retry(ctx, func() error {
		_, err := db.ExecContext(ctx, query)
		return errors.Trace(err)
	})
	if err != nil {
		return fmt.Errorf("failed to run %q pragma: %w", OptimizePragma, err)
	}
	return nil
}

// OptimizeDryRun returns the ANALYZE statements that Optimize would run
// against the database, without running any of them. It writes nothing, so
// it is safe to call at any time.
func OptimizeDryRun(ctx context.Context, db *sql.DB) ([]string, error) {
	query := fmt.Sprintf("PRAGMA %s(0x%x)", OptimizePragma, optimizeDryRunMask)

	var statements []string
	err := runner.Retry(ctx, func() error {
		// Reset on every attempt, so that a retry can not append to the
		// results of a failed one.
		statements = nil

		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return errors.Trace(err)
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var statement string
			if err := rows.Scan(&statement); err != nil {
				return errors.Trace(err)
			}
			statements = append(statements, statement)
		}
		return errors.Trace(rows.Err())
	})
	if err != nil {
		return nil, fmt.Errorf("failed to run %q pragma: %w", OptimizePragma, err)
	}
	return statements, nil
}

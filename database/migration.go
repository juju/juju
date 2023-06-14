// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
)

// stdTxnRunner describes the ability to run a function
// within a standard library SQL transaction.
type stdTxnRunner interface {
	// StdTxn manages the application of a standard library transaction within
	// which the input function is executed.
	// The input context can be used by the caller to cancel this process.
	StdTxn(context.Context, func(context.Context, *sql.Tx) error) error
}

// DBMigration is used to apply a series of deltas to a database.
type DBMigration struct {
	db     stdTxnRunner
	logger Logger
	deltas []database.Delta
}

// NewDBMigration returns a reference to a new migration that
// is used to apply the input deltas to the input database.
// The deltas are applied in the order supplied.
func NewDBMigration(db stdTxnRunner, logger Logger, deltas []database.Delta) *DBMigration {
	return &DBMigration{
		db:     db,
		logger: logger,
		deltas: deltas,
	}
}

// Apply executes all deltas against the database inside a transaction.
func (m *DBMigration) Apply(ctx context.Context) error {
	return m.db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, d := range m.deltas {
			_, err := tx.ExecContext(ctx, d.Stmt(), d.Args()...)
			if err != nil {
				return errors.Trace(err)
			}
		}
		return nil
	})
}

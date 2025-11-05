//go:build dqlite && linux

// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
)

// DeleteDB deletes the dqlite database with the given name from the sql.DB.
func DeleteDB(ctx context.Context, db *sql.DB) error {
	// We unfortunately can't start a BEGIN IMMEDIATE transaction, as the
	// database/sql package does not support it directly. Instead, we're going
	// to start a transaction, then instantly rollback the transaction and
	// start an immediate transaction. This is not ideal, but it works around
	// the limitation.
	// See: https://github.com/mattn/go-sqlite3/issues/400#issuecomment-598953685
	tx, err := beginImmediate(ctx, db)
	if err != nil {
		return errors.Annotatef(err, "starting immediate transaction for deletion")
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA delete_database;"); err != nil {
		_ = tx.Rollback()
		return errors.Annotatef(err, "setting PRAGMA for deletion")
	}
	if err := tx.Commit(); err != nil {
		return errors.Annotatef(err, "committing deletion transaction")
	}

	return nil
}

func beginImmediate(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err == nil {
		_, err = tx.Exec("ROLLBACK; BEGIN IMMEDIATE")
	}
	return tx, err
}

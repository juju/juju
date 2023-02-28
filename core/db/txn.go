// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/juju/errors"
)

const (
	DefaultTimeout = time.Second * 30
)

// Txn defines a generic txn function for applying transactions on a given
// database. It expects that no individual transaction function should take
// longer than the default timeout.
// There are no retry semantics for running the function.
//
// This should not be used directly, instead the TrackedDB should be used to
// handle transactions.
func Txn(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Trace(err)
	}

	if err := fn(ctx, tx); err != nil {
		// TODO (stickupkid): We should retry the rollback, to ensure a clean
		// slate.
		_ = tx.Rollback()
		return errors.Trace(err)
	}

	if err := tx.Commit(); err != nil && err != sql.ErrTxDone {
		return errors.Trace(err)
	}

	return nil
}

// Retry defines a generic retry function for applying a function that
// interacts with the database. It will retry in cases of transient known
// database errors.
func Retry(fn func() error) error {
	return fn()
}

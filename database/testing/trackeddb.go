// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/database/txn"
)

var defaultTransactionRunner = txn.NewTransactionRunner()

// trackedDB is used for testing purposes.
type trackedDB struct {
	db *sql.DB
}

func (t *trackedDB) Txn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return defaultTransactionRunner.Retry(ctx, func() error {
		return errors.Trace(t.TxnNoRetry(ctx, fn))
	})
}
func (t *trackedDB) TxnNoRetry(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return errors.Trace(defaultTransactionRunner.Txn(ctx, t.db, fn))
}

func (t *trackedDB) Err() error {
	return nil
}

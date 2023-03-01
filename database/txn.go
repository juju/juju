// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/juju/juju/database/txn"
)

var (
	defaultTransactioner = txn.NewTransactioner()
)

// Txn defines a generic txn function for applying transactions on a given
// database. It expects that no individual transaction function should take
// longer than the default timeout.
// There are no retry semantics for running the function.
//
// This should not be used directly, instead the TrackedDB should be used to
// handle transactions.
func Txn(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) error {
	return defaultTransactioner.Txn(ctx, db, fn)
}

func Retry(ctx context.Context, fn func() error) error {
	return defaultTransactioner.Retry(ctx, fn)
}

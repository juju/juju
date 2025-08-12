// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// TxnRunner defines an interface for running transactions against a database.
type TxnRunner interface {
	// Txn manages the application of a SQLair transaction within which the
	// input function is executed. See https://github.com/canonical/sqlair.
	// The input context can be used by the caller to cancel this process.
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error

	// StdTxn manages the application of a standard library transaction within
	// which the input function is executed.
	// The input context can be used by the caller to cancel this process.
	StdTxn(context.Context, func(context.Context, *sql.Tx) error) error

	// Dying returns a channel that is closed when the database connection
	// is no longer usable. This can be used to detect when the database is
	// shutting down or has been closed.
	Dying() <-chan struct{}
}

// TxnRunnerFactory aliases a function that
// returns a database.TxnRunner or an error.
type TxnRunnerFactory = func(context.Context) (TxnRunner, error)

// NewTxnRunnerFactoryForNamespace returns a TxnRunnerFactory
// for the input namespaced factory function and namespace.
func NewTxnRunnerFactoryForNamespace[T TxnRunner](f func(context.Context, string) (T, error), ns string) TxnRunnerFactory {
	return func(ctx context.Context) (TxnRunner, error) {
		r, err := f(ctx, ns)
		return r, errors.Capture(err)
	}
}

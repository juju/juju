// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/database/txn"
)

var (
	defaultTransactionRunner = txn.NewRetryingTxnRunner()
)

// Txn executes the input function against the tracked database, using
// the sqlair package. The sqlair package provides a mapping library for
// SQL queries and statements.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
//
// This should not be used directly, instead the TxnRunner should be used to
// handle transactions.
func Txn(ctx context.Context, db *sqlair.DB, fn func(context.Context, *sqlair.TX) error) error {
	return defaultTransactionRunner.Txn(ctx, db, fn)
}

// TxnWithPrecheck runs a transaction with a precheck function that is
// executed before the transaction is started. If the precheck function
// returns an error, the transaction is not started.
//
// Txn executes the input function against the tracked database, using
// the sqlair package. The sqlair package provides a mapping library for
// SQL queries and statements.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
//
// This should not be used directly, instead the TxnRunner should be used to
// handle transactions.
func TxnWithPrecheck(ctx context.Context, db *sqlair.DB, precheck func(context.Context) error, fn func(context.Context, *sqlair.TX) error) error {
	return defaultTransactionRunner.TxnWithPrecheck(ctx, db, precheck, fn)
}

// StdTxn defines a generic txn function for applying transactions on a given
// database. It expects that no individual transaction function should take
// longer than the default timeout.
// There are no retry semantics for running the function.
//
// This should not be used directly, instead the TxnRunner should be used to
// handle transactions.
func StdTxn(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) error {
	return defaultTransactionRunner.StdTxn(ctx, db, fn)
}

// Retry defines a generic retry function for applying transactions on a given
// database. It expects that no individual transaction function should take
// longer than the default timeout.
//
// This should not be used directly, instead the TxnRunner should be used to
// handle transactions.
func Retry(ctx context.Context, fn func(context.Context) error) error {
	return defaultTransactionRunner.Retry(ctx, fn)
}

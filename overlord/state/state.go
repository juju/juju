// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
)

type State struct {
	db *sql.DB
}

func NewState(db *sql.DB) *State {
	return &State{
		db: db,
	}
}

// DB returns the underlying sql.DB.
// It is expected to use the transaction of state, but sometimes a raw database
// is required in some scenarios.
func (s *State) DB() *sql.DB {
	return s.db
}

// PrepareStatement creates a prepared statement for later queries or
// executions.
// Multiple queries or executions may be run concurrently from the
// returned statement.
// The caller must call the statement's Close method
// when the statement is no longer needed.
func (s *State) PrepareStatement(ctx context.Context, query string) (*sql.Stmt, error) {
	return s.db.PrepareContext(ctx, query)
}

type Txn interface {
	// PrepareContext creates a prepared statement for use within a transaction.
	//
	// The returned statement operates within the transaction and will be closed
	// when the transaction has been committed or rolled back.
	//
	// To use an existing prepared statement on this transaction, see Tx.Stmt.
	//
	// The provided context will be used for the preparation of the context, not
	// for the execution of the returned statement. The returned statement
	// will run in the transaction context.
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	// StmtContext returns a transaction-specific prepared statement from
	// an existing statement.
	//
	// Example:
	//  updateMoney, err := db.Prepare("UPDATE balance SET money=money+? WHERE id=?")
	//  ...
	//  tx, err := db.Begin()
	//  ...
	//  res, err := tx.StmtContext(ctx, updateMoney).Exec(123.45, 98293203)
	//
	// The provided context is used for the preparation of the statement, not
	// for the execution of the statement.
	//
	// The returned statement operates within the transaction and will be closed
	// when the transaction has been committed or rolled back.
	StmtContext(context.Context, *sql.Stmt) *sql.Stmt
	// ExecContext executes a query that doesn't return rows.
	// For example: an INSERT and UPDATE.
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	// QueryContext executes a query that returns rows, typically a SELECT.
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

// TxnBuilder allows the building of a series of functions that will be called
// during a transaction commit. Only upon commit is the transaction constructed
// and the function called.
// The functions in the txn builder maybe called multiple times depending on
// how many retries are employed.
type TxnBuilder interface {
	Stage(func(context.Context, Txn) error) TxnBuilder
	Commit() error
}

// Run is a convince function for running one shot transactions, which correctly
// handles the rollback semantics and retries where available.
// The run function maybe called multiple times if the transaction is being
// retried.
func (s *State) Run(fn func(context.Context, Txn) error) error {
	txn, err := s.CreateTxn(context.Background())
	if err != nil {
		return errors.Trace(err)
	}

	return txn.Stage(fn).Commit()
}

// CreateTxn creates a transaction builder. The transaction builder accumulates
// a series of functions that can be executed on a given commit.
func (s *State) CreateTxn(ctx context.Context) (TxnBuilder, error) {
	return &txnBuilder{
		db:  s.DB(),
		ctx: ctx,
	}, nil
}

// txnBuilder creates a type for executing transactions and ensuring rollback
// symantics are employed.
type txnBuilder struct {
	db        *sql.DB
	ctx       context.Context
	runnables []func(context.Context, Txn) error
}

// Context returns the underlying TxnBuilder context.
func (t *txnBuilder) Context() context.Context {
	return t.ctx
}

// Stage adds a function to a given transaction context. The transaction
// isn't committed until the commit method is called.
// The run function maybe called multiple times if the transaction is being
// retried.
func (t *txnBuilder) Stage(fn func(context.Context, Txn) error) TxnBuilder {
	t.runnables = append(t.runnables, fn)
	return t
}

// Commit commits the transaction.
func (t *txnBuilder) Commit() error {
	return withRetry(func() error {
		// Ensure that we don't attempt to retry if the context has been
		// cancelled or errored out.
		if err := t.ctx.Err(); err != nil {
			return errors.Trace(err)
		}

		rawTx, err := t.db.BeginTx(t.ctx, nil)
		if err != nil {
			// Nested transactions are not supported, if we get an error during
			// the begin transaction phase, attempt to rollback both
			// transactions, so that they can correctly start again.
			if rawTx != nil {
				_, _ = rawTx.Exec("ROLLBACK")
			}
			return errors.Trace(err)
		}

		for _, fn := range t.runnables {
			if err := fn(t.ctx, rawTx); err != nil {
				// Ensure we rollback when attempt to run each function with in
				// a transaction commit.
				_ = rawTx.Rollback()
				return errors.Trace(err)
			}
		}
		return rawTx.Commit()
	})
}

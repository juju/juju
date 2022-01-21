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
	// The provided context is used for the preparation of the statement, not for the
	// execution of the statement.
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
type TxnRunner interface {
	WithTxn(func(context.Context, Txn) error) error
	Commit() error
}

func (s *State) BeginTx(ctx context.Context) (TxnRunner, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &txnRunner{
		tx:  tx,
		ctx: ctx,
	}, nil
}

// txnRunner creates a type for executing transactions and ensuring rollback
// symantics are employed.
type txnRunner struct {
	tx  *sql.Tx
	ctx context.Context
}

// WIthTxn runs the function in a given transaction context. The transaction
// isn't committed until the commit method is called.
func (t *txnRunner) WithTxn(fn func(context.Context, Txn) error) error {
	if err := fn(t.ctx, t.tx); err != nil {
		_ = t.tx.Rollback()
		return errors.Trace(err)
	}
	return nil
}

func (t *txnRunner) Commit() error {
	return t.tx.Commit()
}

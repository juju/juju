// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	"sync"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/errors"
)

// TxnRunner is an interface that provides a method for executing a closure
// within the scope of a transaction.
type TxnRunner interface {
	// Txn manages the application of a SQLair transaction within which the
	// input function is executed. See https://github.com/canonical/sqlair.
	// The input context can be used by the caller to cancel this process.
	Txn(context.Context, func(context.Context, *sqlair.TX) error) error
}

// StateBase defines a base struct for requesting a database. This will cache
// the database for the lifetime of the struct.
type StateBase struct {
	dbMutex sync.Mutex
	getDB   database.TxnRunnerFactory
	db      *txnRunner

	// statements is a cache of sqlair statements keyed by the query string.
	statementMutex sync.RWMutex
	statements     map[string]*sqlair.Statement
}

// NewStateBase returns a new StateBase.
func NewStateBase(getDB database.TxnRunnerFactory) *StateBase {
	return &StateBase{
		getDB:      getDB,
		statements: make(map[string]*sqlair.Statement),
	}
}

// DB returns the database for a given namespace.
func (st *StateBase) DB(ctx context.Context) (TxnRunner, error) {
	// Check if the database has already been retrieved.
	st.dbMutex.Lock()
	defer st.dbMutex.Unlock()

	if st.db != nil {
		select {
		case <-st.db.runner.Dying():
			// The database is no longer usable, so we can remove it from the
			// cache and return an error. If the the consumer wants to try
			// again, they can call DB again and it will perform the full
			// retrieval.
			st.db = nil
			return nil, database.ErrDBNotFound
		default:
			// The database is still alive, return it.
			return st.db, nil
		}
	}

	if st.getDB == nil {
		return nil, errors.New("nil getDB")
	}

	db, err := st.getDB(ctx)
	if err != nil {
		return nil, errors.Errorf("invoking getDB: %w", err)
	}
	st.db = &txnRunner{runner: db}
	return st.db, nil
}

// Prepare prepares a SQLair query. If the query has been prepared previously it
// is retrieved from the statement cache.
//
// Note that because the type samples are not considered when retrieving a query
// from the cache, it is possible that two queries may have identical text, but
// use different types. Retrieving the wrong query would result in an error when
// the query was passed the wrong type at execution.
//
// The likelihood of this happening is low since the statement cache is scoped
// to individual domains meaning that the two identically worded statements
// would have to be in the same state package. This issue should be relatively
// rare and caught by QA if present.
func (st *StateBase) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	// Take a read lock to check if the statement is already prepared.
	st.statementMutex.RLock()
	if stmt, ok := st.statements[query]; ok && stmt != nil {
		st.statementMutex.RUnlock()
		return stmt, nil
	}
	st.statementMutex.RUnlock()

	// Grab the write lock to prepare the statement.
	st.statementMutex.Lock()
	defer st.statementMutex.Unlock()

	stmt, err := sqlair.Prepare(query, typeSamples...)
	if err != nil {
		return nil, errors.Errorf("preparing:: %w", err)
	}

	st.statements[query] = stmt
	return stmt, nil
}

// txnRunner is a wrapper around a database.TxnRunner that implements the
// database.TxnRunner interface.
type txnRunner struct {
	runner database.TxnRunner
}

// Txn manages the application of a SQLair transaction within which the
// input function is executed. See https://github.com/canonical/sqlair.
// The input context can be used by the caller to cancel this process.
func (r *txnRunner) Txn(ctx context.Context, fn func(context.Context, *sqlair.TX) error) error {
	return CoerceError(r.runner.Txn(ctx, fn))
}

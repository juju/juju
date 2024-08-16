// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
)

// StateBase defines a base struct for requesting a database. This will cache
// the database for the lifetime of the struct.
type StateBase struct {
	dbMutex sync.RWMutex
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
func (st *StateBase) DB() (database.TxnRunner, error) {
	// Check if the database has already been retrieved.
	// We optimistically check if the database is not nil, before checking
	// if the getDB function is nil. This reduces the branching logic for the
	// common use case.
	st.dbMutex.RLock()
	if st.db != nil {
		db := st.db
		st.dbMutex.RUnlock()
		return db, nil
	}
	st.dbMutex.RUnlock()

	// Move into a write lock to retrieve the database, this should only
	// happen once, so using the full lock isn't a problem.
	st.dbMutex.Lock()
	defer st.dbMutex.Unlock()

	if st.getDB == nil {
		return nil, errors.New("nil getDB")
	}

	db, err := st.getDB()
	if err != nil {
		return nil, errors.Annotate(err, "invoking getDB")
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
		return nil, errors.Annotate(err, "preparing:")
	}

	st.statements[query] = stmt
	return stmt, nil
}

// RunAtomic executes the closure function within the scope of a transaction.
// The closure is passed a AtomicContext that can be passed on to state
// functions, so that they can perform work within that same transaction. The
// closure will be retried according to the transaction retry semantics, if the
// transaction fails due to transient errors. The closure should only be used to
// perform state changes and must not be used to execute queries outside of the
// state scope. This includes performing goroutines or other async operations.
func (st *StateBase) RunAtomic(ctx context.Context, fn func(AtomicContext) error) error {
	db, err := st.DB()
	if err != nil {
		return errors.Annotate(err, "getting database")
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// The atomicContext is created with the transaction and passed to the
		// closure function. This ensures that the transaction is always
		// available to the closure. Once the transaction is complete, the
		// transaction is removed from the atomicContext. This is to prevent the
		// transaction from being used outside of the transaction scope. This
		// will prevent any references to the sqlair.TX from being held outside
		// of the transaction scope.

		// TODO (stickupkid): The atomicContext can be pooled on the StateBase
		// to reduce the number of allocations. Attempting to push the tx into
		// the context would prevent that as a viable option.
		txCtx := &atomicContext{
			Context: ctx,
			tx:      tx,
		}
		defer txCtx.close()

		return fn(txCtx)
	})
}

// AtomicStateBase is an interface that provides a method for executing a
// closure within the scope of a transaction.
type AtomicStateBase interface {
	// RunAtomic executes the closure function within the scope of a
	// transaction. The closure is passed a AtomicContext that can be passed on
	// to state functions, so that they can perform work within that same
	// transaction. The closure will be retried according to the transaction
	// retry semantics, if the transaction fails due to transient errors. The
	// closure should only be used to perform state changes and must not be used
	// to execute queries outside of the state scope. This includes performing
	// goroutines or other async operations.
	RunAtomic(ctx context.Context, fn func(AtomicContext) error) error
}

// Run executes the closure function using the provided AtomicContext as the
// transaction context. It is expected that the closure will perform state
// changes within the transaction scope. Any errors returned from the closure
// are coerced into a standard error to prevent sqlair errors from being
// returned to the Service layer.
func Run(ctx AtomicContext, fn func(context.Context, *sqlair.TX) error) error {
	txCtx, ok := ctx.(*atomicContext)
	if !ok {
		// If you're seeing this error, it means that the atomicContext was not
		// created by RunAtomic. This is a programming error. Did you attempt to
		// wrap the context in a custom context and pass it to Run?
		return fmt.Errorf("programmatic error: AtomicContext is not a *atomicContext: %T", ctx)
	}

	// Ensure that we can lock the context for the duration of the run function.
	// This is to prevent the transaction from being removed from the context
	// or the service layer from attempting to use the transaction outside of
	// the transaction scope.
	txCtx.mu.Lock()
	defer txCtx.mu.Unlock()

	tx := txCtx.tx
	if tx == nil {
		// If you're seeing this error, it means that the AtomicContext was not
		// created by RunAtomic. This is a programming error. Did you capture
		// the AtomicContext from a RunAtomic closure and try to use it outside
		// of the closure?
		return fmt.Errorf("programmatic error: AtomicContext does not have a transaction")
	}

	// Execute the function with the transaction.
	// Coerce the error to ensure that no sql or sqlair errors are returned
	// from the function and into the Service layer.
	return CoerceError(fn(ctx, tx))
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

// StdTxn manages the application of a standard library transaction within
// which the input function is executed.
// The input context can be used by the caller to cancel this process.
func (r *txnRunner) StdTxn(ctx context.Context, fn func(context.Context, *sql.Tx) error) error {
	return CoerceError(r.runner.StdTxn(ctx, fn))
}

// AtomicContext is a typed context that provides access to the database transaction
// for the duration of a transaction.
type AtomicContext interface {
	context.Context
}

// atomicContext is the concrete implementation of the AtomicContext interface.
// The atomicContext ensures that a transaction is always available to during
// the execution of a transaction. The atomicContext stores the sqlair.TX
// directly on the struct to prevent the need to fork the context during the
// transaction. The mutex prevents data-races when the transaction is removed
// from the context.
type atomicContext struct {
	context.Context

	mu sync.Mutex
	tx *sqlair.TX
}

func (c *atomicContext) close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tx = nil
}

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
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
	db      database.TxnRunner

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

	var err error
	if st.db, err = st.getDB(); err != nil {
		return nil, errors.Annotate(err, "invoking getDB")
	}
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

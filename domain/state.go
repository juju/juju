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
	mu    sync.Mutex
	getDB database.TxnRunnerFactory
	db    database.TxnRunner

	// stmts is a cache of sqlair statements keyed by the query string.
	stmts     map[string]*sqlair.Statement
	stmtMutex sync.RWMutex
}

// NewStateBase returns a new StateBase.
func NewStateBase(getDB database.TxnRunnerFactory) *StateBase {
	return &StateBase{
		getDB: getDB,
		stmts: make(map[string]*sqlair.Statement),
	}
}

// DB returns the database for a given namespace.
func (st *StateBase) DB() (database.TxnRunner, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.getDB == nil {
		return nil, errors.New("nil getDB")
	}

	if st.db == nil {
		var err error
		if st.db, err = st.getDB(); err != nil {
			return nil, errors.Annotate(err, "invoking getDB")
		}
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
	st.stmtMutex.RLock()
	if stmt, ok := st.stmts[query]; ok && stmt != nil {
		st.stmtMutex.RUnlock()
		return stmt, nil
	}
	st.stmtMutex.RUnlock()
	// Grab the write lock to prepare the statement.
	st.stmtMutex.Lock()
	defer st.stmtMutex.Unlock()
	stmt, err := sqlair.Prepare(query, typeSamples...)
	if err != nil {
		return nil, errors.Annotate(err, "preparing:")
	}
	st.stmts[query] = stmt
	return stmt, nil
}

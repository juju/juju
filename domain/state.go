// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
)

// DBFactory defines a function that returns a database or an error if a
// database cannot be returned.
type DBFactory = func() (database.TrackedDB, error)

// NamespaceDBFactory returns a DBFactory that returns a database from the
// given getter function.
func NamespaceDBFactory(getter func(string) (database.TrackedDB, error), namespace string) DBFactory {
	return func() (database.TrackedDB, error) {
		if getter == nil {
			return nil, errors.New("nil getter")
		}

		db, err := getter(namespace)
		return db, errors.Trace(err)
	}
}

// TrackingDBFactory returns a DBFactory that returns the given database.
func TrackedDBFactory(db database.TrackedDB) DBFactory {
	return func() (database.TrackedDB, error) {
		return db, nil
	}
}

// StateBase defines a base struct for requesting a database. This will cache
// the database for the lifetime of the struct.
type StateBase struct {
	mu    sync.Mutex
	getDB DBFactory
	db    database.TrackedDB
}

// NewStateBase returns a new StateBase.
func NewStateBase(getDB DBFactory) *StateBase {
	return &StateBase{
		getDB: getDB,
	}
}

// DB returns the database for a given namespace.
func (st *StateBase) DB() (database.TrackedDB, error) {
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

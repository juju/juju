// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
)

// TxnRunnerFactory aliases a function that
// returns a database.TxnRunner or an error.
type TxnRunnerFactory = func() (database.TxnRunner, error)

// NewTxnRunnerFactory returns a TxnRunnerFactory for the input
// changestream.WatchableDB factory function.
// This ensures that we never pass the ability to access the
// change-stream into a state object.
// State objects should only be concerned with persistence and retrieval.
// Watchers are the concern of the service layer.
func NewTxnRunnerFactory(f func() (changestream.WatchableDB, error)) TxnRunnerFactory {
	return func() (database.TxnRunner, error) {
		r, err := f()
		return r, errors.Trace(err)
	}
}

// NewTxnRunnerFactoryForNamespace returns a TxnRunnerFactory for the input
// namespaced changestream.WatchableDB factory function and namespace.
func NewTxnRunnerFactoryForNamespace(f func(string) (changestream.WatchableDB, error), ns string) TxnRunnerFactory {
	return func() (database.TxnRunner, error) {
		r, err := f(ns)
		return r, errors.Trace(err)
	}
}

// StateBase defines a base struct for requesting a database. This will cache
// the database for the lifetime of the struct.
type StateBase struct {
	mu    sync.Mutex
	getDB TxnRunnerFactory
	db    database.TxnRunner
}

// NewStateBase returns a new StateBase.
func NewStateBase(getDB TxnRunnerFactory) *StateBase {
	return &StateBase{
		getDB: getDB,
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

// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
)

type DBFactory = func() (database.TrackedDB, error)

func DBFac(getter func(string) (database.TrackedDB, error), namespace string) DBFactory {
	return func() (database.TrackedDB, error) {
		db, err := getter(namespace)
		return db, errors.Trace(err)
	}
}

type StateBase struct {
	getDB DBFactory
	db    database.TrackedDB
}

func NewStateBase(getDB DBFactory) *StateBase {
	return &StateBase{getDB: getDB}
}

func (st *StateBase) DB() (database.TrackedDB, error) {
	if st.getDB == nil {
		return nil, errors.New("nil getDB")
	}

	var err error
	if st.db == nil {
		if st.db, err = st.getDB(); err != nil {
			return nil, errors.Annotate(err, "invoking getDB")
		}
	}

	return st.db, nil
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"errors"

	"labix.org/v2/mgo"
)

var DocNotFound = errors.New("not found")

type stateStorage struct {
	coll   *mgo.Collection
	closer func()
	stage  interface{}
}

func NewStateStorage(st *State, coll string) *stateStorage {
	collection, closer := st.getCollection(coll)
	stor := stateStorage{
		coll:   collection,
		closer: closer,
	}
	return &stor
}

func (s *stateStorage) AddDoc(id string, doc interface{}) error {
	_, err := s.coll.UpsertId(id, doc)
	return err
}

func (s *stateStorage) Doc(id string, doc interface{}) error {
	query := s.coll.FindId(id)
	size, err := query.Count()
	if err != nil {
		return err
	}
	if size == 0 {
		return DocNotFound
	}
	// There can only be one!
	err = query.One(doc)
	return err
}

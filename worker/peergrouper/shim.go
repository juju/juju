// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"github.com/juju/replicaset"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state"
)

// This file holds code that translates from State
// to the interface expected internally by the
// worker.

type StateShim struct {
	*state.State
}

func (s StateShim) Machine(id string) (Machine, error) {
	return s.State.Machine(id)
}

func (s StateShim) Space(name string) (Space, error) {
	return s.State.Space(name)
}

// MongoSessionShim wraps a *mgo.Session to conform to the
// MongoSession interface.
type MongoSessionShim struct {
	*mgo.Session
}

func (s MongoSessionShim) CurrentStatus() (*replicaset.Status, error) {
	return replicaset.CurrentStatus(s.Session)
}

func (s MongoSessionShim) CurrentMembers() ([]replicaset.Member, error) {
	return replicaset.CurrentMembers(s.Session)
}

func (s MongoSessionShim) Set(members []replicaset.Member) error {
	return replicaset.Set(s.Session, members)
}

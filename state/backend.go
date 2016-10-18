// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/mongo"
)

// modelBackend collects together some useful internal state methods for
// accessing mongo and mapping local and global ids to one another.
//
// Its primary purpose is to insulate watcher implementations from the
// other features of state (specifically, to deny access to the .workers
// field and prevent a class of bug in which it tries to use two
// different underlying TxnWatchers), but it's probably also useful
// elsewhere.
type modelBackend interface {
	docID(string) string
	localID(string) string
	strictLocalID(string) (string, error)
	getCollection(name string) (mongo.Collection, func())
	getCollectionFor(modelUUID, name string) (mongo.Collection, func())
}

// isLocalID returns a watcher filter func that rejects ids not specific
// to the supplied modelBackend.
func isLocalID(st modelBackend) func(interface{}) bool {
	return func(id interface{}) bool {
		key, ok := id.(string)
		if !ok {
			return false
		}
		_, err := st.strictLocalID(key)
		return err == nil
	}
}

// docID generates a globally unique id value
// where the model uuid is prefixed to the
// localID.
func (st *State) docID(localID string) string {
	return ensureModelUUID(st.ModelUUID(), localID)
}

// localID returns the local id value by stripping
// off the model uuid prefix if it is there.
func (st *State) localID(ID string) string {
	modelUUID, localID, ok := splitDocID(ID)
	if !ok || modelUUID != st.ModelUUID() {
		return ID
	}
	return localID
}

// strictLocalID returns the local id value by removing the
// model UUID prefix.
//
// If there is no prefix matching the State's model, an error is
// returned.
func (st *State) strictLocalID(ID string) (string, error) {
	modelUUID, localID, ok := splitDocID(ID)
	if !ok || modelUUID != st.ModelUUID() {
		return "", errors.Errorf("unexpected id: %#v", ID)
	}
	return localID, nil
}

// getCollection fetches a named collection using a new session if the
// database has previously been logged in to. It returns the
// collection and a closer function for the session.
//
// If the collection stores documents for multiple models, the
// returned collection will automatically perform model
// filtering where possible. See modelStateCollection below.
func (st *State) getCollection(name string) (mongo.Collection, func()) {
	return st.database.GetCollection(name)
}

func (st *State) getCollectionFor(modelUUID, name string) (mongo.Collection, func()) {
	database, dbcloser := st.database.CopyForModel(modelUUID)
	collection, closer := database.GetCollection(name)
	return collection, func() {
		closer()
		dbcloser()
	}
}

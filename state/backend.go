// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

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

// Passive is intended to hold the backing functionality necessary for
// "most" of state: the stuff that's reading or writing the database.
//
// At the moment it has very few methods; we intend to thread it through
// state step by step, moving simple methods from State (which holds all
// the active, difficult bits like watchers and lease managers) with the
// intent of eventually isolating those into a separate session-hungry
// type that can be restricted to 1-per-controller; leaving the Passive-
// only parts easier to manage on a per-connection? per-request? basis.
type Passive struct {
	modelUUID string
	database  Database
}

// ModelTag() returns the model tag for the model controlled by
// this state instance.
func (st *Passive) ModelTag() names.ModelTag {
	return names.NewModelTag(st.modelUUID)
}

// ModelUUID returns the model UUID for the model
// controlled by this state instance.
func (st *Passive) ModelUUID() string {
	return st.modelUUID
}

// docID generates a globally unique id value
// where the model uuid is prefixed to the
// localID.
func (st *Passive) docID(localID string) string {
	return ensureModelUUID(st.modelUUID, localID)
}

// localID returns the local id value by stripping
// off the model uuid prefix if it is there.
func (st *Passive) localID(ID string) string {
	modelUUID, localID, ok := splitDocID(ID)
	if !ok || modelUUID != st.modelUUID {
		return ID
	}
	return localID
}

// strictLocalID returns the local id value by removing the
// model UUID prefix.
//
// If there is no prefix matching the State's model, an error is
// returned.
func (st *Passive) strictLocalID(ID string) (string, error) {
	modelUUID, localID, ok := splitDocID(ID)
	if !ok || modelUUID != st.modelUUID {
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
func (st *Passive) getCollection(name string) (mongo.Collection, func()) {
	return st.database.GetCollection(name)
}

// readTxnRevno is a convenience method delegating to the state's Database.
func (st *Passive) readTxnRevno(collectionName string, id interface{}) (int64, error) {
	collection, closer := st.database.GetCollection(collectionName)
	defer closer()
	query := collection.FindId(id).Select(bson.D{{"txn-revno", 1}})
	var result struct {
		TxnRevno int64 `bson:"txn-revno"`
	}
	err := query.One(&result)
	return result.TxnRevno, errors.Trace(err)
}

// runTransaction is a convenience method delegating to the state's Database.
func (st *Passive) runTransaction(ops []txn.Op) error {
	runner, closer := st.database.TransactionRunner()
	defer closer()
	return runner.RunTransaction(ops)
}

// runRawTransaction is a convenience method that will run a single
// transaction using a "raw" transaction runner that won't perform
// model filtering.
func (st *Passive) runRawTransaction(ops []txn.Op) error {
	runner, closer := st.database.TransactionRunner()
	defer closer()
	if multiRunner, ok := runner.(*multiModelRunner); ok {
		runner = multiRunner.rawRunner
	}
	return runner.RunTransaction(ops)
}

// run is a convenience method delegating to the state's Database.
func (st *Passive) run(transactions jujutxn.TransactionSource) error {
	runner, closer := st.database.TransactionRunner()
	defer closer()
	return runner.Run(transactions)
}

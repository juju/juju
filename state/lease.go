// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/lease"
)

type leaseEntity struct {
	LastUpdate time.Time
	lease.Token
}

// NewLeasePersistor returns a new LeasePersistor. It should be passed
// functions it can use to run transactions and get collections.
func NewLeasePersistor(
	collectionName string,
	runTransaction func([]txn.Op) error,
	getCollection func(string) (_ stateCollection, closer func()),
) *LeasePersistor {
	return &LeasePersistor{
		collectionName: collectionName,
		runTransaction: runTransaction,
		getCollection:  getCollection,
	}
}

// LeasePersistor represents logic which can persist lease tokens to a
// data store.
type LeasePersistor struct {
	collectionName string
	runTransaction func([]txn.Op) error
	getCollection  func(string) (_ stateCollection, closer func())
}

// WriteToken writes the given token to the data store with the given
// ID.
func (p *LeasePersistor) WriteToken(id string, tok lease.Token) error {

	entity := leaseEntity{time.Now(), tok}

	// Write's should always overwrite anything that's there. The
	// business-logic of managing leases is handled elsewhere.
	ops := []txn.Op{
		// First remove anything that's there.
		{
			C:      p.collectionName,
			Id:     id,
			Remove: true,
		},
		// Then insert the token.
		{
			Assert: txn.DocMissing,
			C:      p.collectionName,
			Id:     id,
			Insert: entity,
		},
	}

	if err := p.runTransaction(ops); err != nil {
		return errors.Annotatef(err, `could not add token "%s" to data-store`, tok.Id)
	}

	return nil
}

// RemoveToken removes the lease token with the given ID from the data
// store.
func (p *LeasePersistor) RemoveToken(id string) error {

	ops := []txn.Op{{C: p.collectionName, Id: id, Remove: true}}
	if err := p.runTransaction(ops); err != nil {
		return errors.Annotatef(err, `could not remove token "%s"`, id)
	}

	return nil
}

// PersistedTokens retrieves all tokens currently persisted.
func (p *LeasePersistor) PersistedTokens() (tokens []lease.Token, _ error) {

	collection, closer := p.getCollection(p.collectionName)
	defer closer()

	// Pipeline entities into tokens.
	var query bson.D
	iter := collection.Find(query).Iter()
	defer iter.Close()

	var doc leaseEntity
	for iter.Next(&doc) {
		tokens = append(tokens, doc.Token)
	}

	if err := iter.Err(); err != nil {
		return nil, errors.Annotate(err, "could not retrieve all persisted tokens")
	}

	return tokens, nil
}

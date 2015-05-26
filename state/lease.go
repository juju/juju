// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/lease"
)

// leaseEntity represents a lease in mongo.
type leaseEntity struct {
	LastUpdate  time.Time `bson:"lastupdate"`
	lease.Token `bson:"token"`

	// TxnRevNo is used to ensure no other client has
	// updated the same lease while a write is being done.
	TxnRevno int64 `bson:"txn-revno"`
}

// NewLeasePersistor returns a new LeasePersistor. It should be passed
// functions it can use to run transactions and get collections.
func NewLeasePersistor(
	collectionName string,
	runTransaction func(jujutxn.TransactionSource) error,
	getCollection func(string) (_ stateCollection, closer func()),
) *LeasePersistor {
	getLeaseCollection := func(name string) (_ leaseCollection, closer func()) {
		sc, closer := getCollection(name)
		return &genericLeaseCollection{sc}, closer
	}
	return &LeasePersistor{
		collectionName: collectionName,
		runTransaction: runTransaction,
		getCollection:  getLeaseCollection,
	}
}

// LeasePersistor represents logic which can persist lease tokens to a
// data store.
type LeasePersistor struct {
	collectionName string
	runTransaction func(jujutxn.TransactionSource) error
	getCollection  func(string) (_ leaseCollection, closer func())
}

// leaseCollection provides bespoke lease methods on top of a standard
// state collection.
type leaseCollection interface {
	stateCollection

	// FindById finds the lease with the specified id.
	FindById(id string) (*leaseEntity, error)
}

type genericLeaseCollection struct {
	stateCollection
}

// FindById finds the lease with the specified id.
func (lf *genericLeaseCollection) FindById(id string) (*leaseEntity, error) {
	var lease leaseEntity
	if err := lf.FindId(id).One(&lease); err != nil {
		return nil, err
	}
	return &lease, nil
}

// WriteToken writes the given token to the data store with the given
// ID.
func (p *LeasePersistor) WriteToken(id string, tok lease.Token) error {

	collection, closer := p.getCollection(p.collectionName)
	defer closer()

	// TODO(wallyworld) - this logic is a stop-gap until a proper refactoring is done
	// We'll be especially paranoid here - to avoid potentially overwriting lease info
	// from another client, if the txn fails to apply, we'll abort instead of retrying.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			return nil, errors.New("simultaneous lease updates occurred")
		}
		existing, err := collection.FindById(id)
		if err == mgo.ErrNotFound {
			entity := leaseEntity{LastUpdate: time.Now(), Token: tok}
			return []txn.Op{
				{
					C:      p.collectionName,
					Id:     id,
					Assert: txn.DocMissing,
					Insert: entity,
				},
			}, nil
		} else if err != nil {
			return nil, errors.Annotatef(err, "reading existing lease for token %v", tok)
		}
		return []txn.Op{
			{
				C:      p.collectionName,
				Id:     id,
				Assert: bson.D{{"txn-revno", existing.TxnRevno}},
				Update: bson.M{"$set": bson.M{"lastupdate": time.Now(), "token": tok}},
			},
		}, nil
	}
	if err := p.runTransaction(buildTxn); err != nil {
		return errors.Annotatef(err, `could not add token "%s" to data-store`, tok.Id)
	}

	return nil
}

// RemoveToken removes the lease token with the given ID from the data
// store.
func (p *LeasePersistor) RemoveToken(id string) error {

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt == 0 {
			return []txn.Op{
				{
					C:      p.collectionName,
					Id:     id,
					Assert: txn.DocExists,
					Remove: true,
				},
			}, nil
		}
		return nil, jujutxn.ErrNoOperations
	}
	if err := p.runTransaction(buildTxn); err != nil {
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

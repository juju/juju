// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/lease"
)

const (
	testCollectionName = "test collection"
	testNamespace      = "leadership-stub-service"
	testId             = "stub-unit/0"
	testDuration       = 30 * time.Hour
)

var (
	_ = gc.Suite(&leaseSuite{})
)

//
// Stub functions for when we don't care.
//

func stubRunTransaction(txns jujutxn.TransactionSource) error {
	return nil
}

func stubGetCollection(collectionName string) (stateCollection, func()) {
	return &genericStateCollection{}, func() {}
}

type stubLeaseCollection struct {
	stateCollection
	tokenToReturn *leaseEntity
}

func (s *stubLeaseCollection) FindById(id string) (*leaseEntity, error) {
	if s.tokenToReturn == nil {
		return nil, mgo.ErrNotFound
	}
	return s.tokenToReturn, nil
}

type leaseSuite struct{}

func (s *leaseSuite) TestWriteNewToken(c *gc.C) {

	tok := lease.Token{testNamespace, testId, time.Now().Add(testDuration)}

	stubRunTransaction := func(txns jujutxn.TransactionSource) error {
		ops, err := txns(0)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(ops[0].Assert, gc.Equals, txn.DocMissing)
		c.Check(ops[0].C, gc.Equals, testCollectionName)
		c.Check(ops[0].Insert.(leaseEntity).Token, gc.DeepEquals, tok)
		c.Check(ops[0].Id, gc.Equals, testId)
		return nil
	}

	closerCallCount := 0
	stubGetCollection := func(collectionName string) (leaseCollection, func()) {
		c.Check(collectionName, gc.Equals, testCollectionName)
		return &stubLeaseCollection{&genericStateCollection{}, nil}, func() { closerCallCount++ }
	}
	persistor := LeasePersistor{testCollectionName, stubRunTransaction, stubGetCollection}
	err := persistor.WriteToken(testId, tok)
	c.Assert(err, gc.IsNil)
	c.Assert(closerCallCount, gc.Equals, 1)
}

func (s *leaseSuite) TestWriteTokenReplaceExisting(c *gc.C) {

	tok := lease.Token{testNamespace, testId, time.Now().Add(testDuration)}
	stubRunTransaction := func(txns jujutxn.TransactionSource) error {
		ops, err := txns(0)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(ops[0].Assert, gc.DeepEquals, bson.D{bson.DocElem{Name: "txn-revno", Value: int64(10)}})
		c.Check(ops[0].C, gc.Equals, testCollectionName)
		c.Check(ops[0].Id, gc.Equals, testId)

		values := ops[0].Update.(bson.M)["$set"].(bson.M)
		token := values["token"].(lease.Token)
		c.Assert(token, gc.DeepEquals, tok)
		return nil
	}

	existingTok := lease.Token{testNamespace, "1234", time.Now().Add(testDuration)}
	existing := leaseEntity{time.Now(), existingTok, 10}
	closerCallCount := 0
	stubGetCollection := func(collectionName string) (leaseCollection, func()) {
		c.Check(collectionName, gc.Equals, testCollectionName)
		return &stubLeaseCollection{&genericStateCollection{}, &existing}, func() { closerCallCount++ }
	}
	persistor := LeasePersistor{testCollectionName, stubRunTransaction, stubGetCollection}
	err := persistor.WriteToken(testId, tok)
	c.Assert(err, gc.IsNil)
	c.Assert(closerCallCount, gc.Equals, 1)
}

func (s *leaseSuite) TestWriteTokenConflict(c *gc.C) {

	tok := lease.Token{testNamespace, testId, time.Now().Add(testDuration)}

	stubRunTransaction := func(txns jujutxn.TransactionSource) error {
		// Any attempt to build txns with attempt>1 is rejected.
		_, err := txns(1)
		c.Assert(err, gc.NotNil)
		return err
	}

	closerCallCount := 0
	stubGetCollection := func(collectionName string) (leaseCollection, func()) {
		c.Check(collectionName, gc.Equals, testCollectionName)
		return &stubLeaseCollection{&genericStateCollection{}, nil}, func() { closerCallCount++ }
	}
	persistor := LeasePersistor{testCollectionName, stubRunTransaction, stubGetCollection}
	err := persistor.WriteToken(testId, tok)
	c.Assert(err, gc.NotNil)
	c.Assert(closerCallCount, gc.Equals, 1)
}

func (s *leaseSuite) TestRemoveToken(c *gc.C) {

	stubRunTransaction := func(txns jujutxn.TransactionSource) error {
		ops, err := txns(0)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ops, gc.HasLen, 1)

		c.Check(ops[0].C, gc.Equals, testCollectionName)
		c.Check(ops[0].Remove, gc.Equals, true)
		c.Check(ops[0].Id, gc.Equals, testId)

		_, err = txns(1)
		c.Assert(err, gc.Equals, jujutxn.ErrNoOperations)

		return nil
	}

	persistor := NewLeasePersistor(testCollectionName, stubRunTransaction, stubGetCollection)
	err := persistor.RemoveToken(testId)

	c.Assert(err, gc.IsNil)
}

func (s *leaseSuite) TestPersistedTokens(c *gc.C) {

	closerCallCount := 0
	stubGetCollection := func(collectionName string) (stateCollection, func()) {
		c.Check(collectionName, gc.Equals, testCollectionName)
		return &genericStateCollection{}, func() { closerCallCount++ }
	}

	persistor := NewLeasePersistor(testCollectionName, stubRunTransaction, stubGetCollection)

	// PersistedTokens will panic when it tries to use the empty collection.
	defer func() {
		r := recover()
		c.Assert(r, gc.NotNil)

		switch closerCallCount {
		case 1:
		case 0:
			c.Errorf("The closer for the collection was never called.")
		default:
			c.Errorf("The closer for the collection was called too many times (%d).", closerCallCount)
		}
	}()

	persistor.PersistedTokens()
}

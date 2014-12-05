// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
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

func stubRunTransaction(ops []txn.Op) error {
	return nil
}

func stubGetCollection(collectionName string) (*mgo.Collection, func()) {
	return &mgo.Collection{}, func() {}
}

type leaseSuite struct{}

func (s *leaseSuite) TestWriteToken(c *gc.C) {

	tok := lease.Token{testNamespace, testId, time.Now().Add(testDuration)}

	stubRunTransaction := func(ops []txn.Op) error {
		c.Assert(ops, gc.HasLen, 2)

		// First delete.
		c.Check(ops[0].C, gc.Equals, testCollectionName)
		c.Check(ops[0].Remove, gc.Equals, true)
		c.Check(ops[0].Id, gc.Equals, testId)

		// Then insert.
		c.Check(ops[1].Assert, gc.Equals, txn.DocMissing)
		c.Check(ops[1].C, gc.Equals, testCollectionName)
		c.Check(ops[1].Insert.(leaseEntity).Token, gc.DeepEquals, tok)
		c.Check(ops[1].Id, gc.Equals, testId)

		return nil
	}

	persistor := NewLeasePersistor(testCollectionName, stubRunTransaction, stubGetCollection)

	err := persistor.WriteToken(testId, tok)

	c.Assert(err, gc.IsNil)
}

func (s *leaseSuite) TestRemoveToken(c *gc.C) {

	stubRunTransaction := func(ops []txn.Op) error {
		c.Assert(ops, gc.HasLen, 1)

		c.Check(ops[0].C, gc.Equals, testCollectionName)
		c.Check(ops[0].Remove, gc.Equals, true)
		c.Check(ops[0].Id, gc.Equals, testId)

		return nil
	}

	persistor := NewLeasePersistor(testCollectionName, stubRunTransaction, stubGetCollection)
	err := persistor.RemoveToken(testId)

	c.Assert(err, gc.IsNil)
}

func (s *leaseSuite) TestPersistedTokens(c *gc.C) {

	closerCallCount := 0
	stubGetCollection := func(collectionName string) (*mgo.Collection, func()) {
		c.Check(collectionName, gc.Equals, testCollectionName)
		return &mgo.Collection{}, func() { closerCallCount++ }
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

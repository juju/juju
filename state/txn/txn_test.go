// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn_test

import (
	gitjujutesting "github.com/juju/testing"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	gc "launchpad.net/gocheck"

	statetxn "github.com/juju/juju/state/txn"
	txntesting "github.com/juju/juju/state/txn/testing"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&txnSuite{})

type txnSuite struct {
	testing.BaseSuite
	gitjujutesting.MgoSuite
	collection *mgo.Collection
	txnRunner  statetxn.Runner
}

func (s *txnSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *txnSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.BaseSuite.TearDownSuite(c)
}

func (s *txnSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	db := s.Session.DB("juju")
	s.collection = db.C("test")
	s.txnRunner = statetxn.NewRunner(txn.NewRunner(s.collection))
}

func (s *txnSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

type simpleDoc struct {
	Id   string `bson:"_id"`
	Name string
}

func (s *txnSuite) TestRunTransaction(c *gc.C) {
	doc := simpleDoc{"1", "Foo"}
	ops := []txn.Op{{
		C:      s.collection.Name,
		Id:     doc.Id,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
	err := s.txnRunner.runTransaction(ops)
	c.Assert(err, gc.IsNil)
	var found simpleDoc
	err = s.collection.FindId("1").One(&found)
	c.Assert(err, gc.IsNil)
	c.Assert(found, gc.DeepEquals, doc)
}

func (s *txnSuite) TestRun(c *gc.C) {
	doc := simpleDoc{"1", "Foo"}
	maxAttempt := 0
	builtTxn := func(attempt int) (ops []txn.Op, err error) {
		maxAttempt = attempt
		ops = []txn.Op{{
			C:      s.collection.Name,
			Id:     doc.Id,
			Assert: txn.DocMissing,
			Insert: doc,
		}}
		return ops, nil
	}
	err := s.txnRunner.run(builtTxn)
	c.Assert(err, gc.IsNil)
	var found simpleDoc
	err = s.collection.FindId("1").One(&found)
	c.Assert(err, gc.IsNil)
	c.Assert(maxAttempt, gc.Equals, 1)
	c.Assert(found, gc.DeepEquals, doc)
}

func (s *txnSuite) setDocName(c *gc.C, id, name string) {
	ops := []txn.Op{{
		C:      s.collection.Name,
		Id:     id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"name", name}}}},
	}}
	err := s.txnRunner.runTransaction(ops)
	c.Assert(err, gc.IsNil)
}

func (s *txnSuite) insertDoc(c *gc.C, id, name string) {
	doc := simpleDoc{id, name}
	ops := []txn.Op{{
		C:      s.collection.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
	err := s.txnRunner.runTransaction(ops)
	c.Assert(err, gc.IsNil)
}

func (s *txnSuite) TestBeforeHooks(c *gc.C) {
	s.insertDoc(c, "1", "Simple")
	changeFuncs := []func(){
		func() { s.setDocName(c, "1", "FooBar") },
		func() { s.setDocName(c, "1", "Foo") },
	}
	defer txntesting.SetBeforeHooks(c, s.txnRunner, changeFuncs...).Check()
	maxAttempt := 0
	builtTxn := func(attempt int) (ops []txn.Op, err error) {
		maxAttempt = attempt
		ops = []txn.Op{{
			C:      s.collection.Name,
			Id:     "1",
			Assert: bson.D{{"name", "Foo"}},
			Update: bson.D{{"$set", bson.D{{"name", "Bar"}}}},
		}}
		return ops, nil
	}
	err := s.txnRunner.run(builtTxn)
	c.Assert(err, gc.IsNil)
	var found simpleDoc
	err = s.collection.FindId("1").One(&found)
	c.Assert(err, gc.IsNil)
	c.Assert(maxAttempt, gc.Equals, 2)
	doc := simpleDoc{"1", "Bar"}
	c.Assert(found, gc.DeepEquals, doc)
}

func (s *txnSuite) TestAfterHooks(c *gc.C) {
	changeFuncs := []func(){
		func() { s.insertDoc(c, "1", "Foo") },
	}
	defer txntesting.SetAfterHooks(c, s.txnRunner, changeFuncs...).Check()
	maxAttempt := 0
	builtTxn := func(attempt int) (ops []txn.Op, err error) {
		maxAttempt = attempt
		ops = []txn.Op{{
			C:      s.collection.Name,
			Id:     "1",
			Assert: bson.D{{"name", "Foo"}},
			Update: bson.D{{"$set", bson.D{{"name", "Bar"}}}},
		}}
		return ops, nil
	}
	err := s.txnRunner.run(builtTxn)
	c.Assert(err, gc.IsNil)
	var found simpleDoc
	err = s.collection.FindId("1").One(&found)
	c.Assert(err, gc.IsNil)
	c.Assert(maxAttempt, gc.Equals, 2)
	doc := simpleDoc{"1", "Bar"}
	c.Assert(found, gc.DeepEquals, doc)
}

func (s *txnSuite) TestRetryHooks(c *gc.C) {
	s.insertDoc(c, "1", "Foo")
	defer txntesting.SetRetryHooks(c, s.txnRunner, func() {
		s.setDocName(c, "1", "Bar")
	}, func() {
		s.setDocName(c, "1", "Foo")
	}).Check()

	maxAttempt := 0
	builtTxn := func(attempt int) (ops []txn.Op, err error) {
		maxAttempt = attempt
		ops = []txn.Op{{
			C:      s.collection.Name,
			Id:     "1",
			Assert: bson.D{{"name", "Foo"}},
			Update: bson.D{{"$set", bson.D{{"name", "FooBar"}}}},
		}}
		return ops, nil
	}
	err := s.txnRunner.run(builtTxn)
	c.Assert(err, gc.IsNil)
	c.Assert(maxAttempt, gc.Equals, 3)
	var found simpleDoc
	err = s.collection.FindId("1").One(&found)
	c.Assert(err, gc.IsNil)
	doc := simpleDoc{"1", "FooBar"}
	c.Assert(found, gc.DeepEquals, doc)
}

func (s *txnSuite) TestExcessiveContention(c *gc.C) {
	maxAttempt := 0
	builtTxn := func(attempt int) (ops []txn.Op, err error) {
		maxAttempt = attempt
		ops = []txn.Op{{
			C:      s.collection.Name,
			Id:     "1",
			Assert: bson.D{{"name", "Foo"}},
			Update: bson.D{{"$set", bson.D{{"name", "Bar"}}}},
		}}
		return ops, nil
	}
	err := s.txnRunner.run(builtTxn)
	c.Assert(err, gc.Equals, statetxn.ErrExcessiveContention)
	c.Assert(maxAttempt, gc.Equals, 3)
}

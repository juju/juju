// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
)

var _ = gc.Suite(&sequenceSuite{})

type sequenceSuite struct {
	ConnSuite
}

func (s *sequenceSuite) TestSequence(c *gc.C) {
	s.incAndCheck(c, s.State, "foo", 0)
	s.checkDocCount(c, 1)
	s.checkDoc(c, s.State.EnvironUUID(), "foo", 1)

	s.incAndCheck(c, s.State, "foo", 1)
	s.checkDocCount(c, 1)
	s.checkDoc(c, s.State.EnvironUUID(), "foo", 2)
}

func (s *sequenceSuite) TestMultipleSequences(c *gc.C) {
	s.incAndCheck(c, s.State, "foo", 0)
	s.incAndCheck(c, s.State, "bar", 0)
	s.incAndCheck(c, s.State, "bar", 1)
	s.incAndCheck(c, s.State, "foo", 1)
	s.incAndCheck(c, s.State, "bar", 2)

	s.checkDocCount(c, 2)
	s.checkDoc(c, s.State.EnvironUUID(), "foo", 2)
	s.checkDoc(c, s.State.EnvironUUID(), "bar", 3)
}

func (s *sequenceSuite) TestSequenceWithMultipleEnvs(c *gc.C) {
	state1 := s.State
	state2 := s.Factory.MakeEnvironment(c, nil)
	defer state2.Close()

	s.incAndCheck(c, state1, "foo", 0)
	s.incAndCheck(c, state2, "foo", 0)
	s.incAndCheck(c, state1, "foo", 1)
	s.incAndCheck(c, state2, "foo", 1)
	s.incAndCheck(c, state1, "foo", 2)

	s.checkDocCount(c, 2)
	s.checkDoc(c, state1.EnvironUUID(), "foo", 3)
	s.checkDoc(c, state2.EnvironUUID(), "foo", 2)
}

func (s *sequenceSuite) incAndCheck(c *gc.C, st *state.State, name string, expectedCount int) {
	value, err := state.Sequence(st, name)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(value, gc.Equals, expectedCount)
}

func (s *sequenceSuite) checkDocCount(c *gc.C, expectedCount int) {
	coll, closer := state.GetRawCollection(s.State, "sequence")
	defer closer()
	count, err := coll.Count()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, expectedCount)
}

func (s *sequenceSuite) checkDoc(c *gc.C, envUUID, name string, value int) {
	coll, closer := state.GetRawCollection(s.State, "sequence")
	defer closer()

	docID := envUUID + ":" + name
	var doc bson.M
	err := coll.FindId(docID).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(doc["_id"], gc.Equals, docID)
	c.Check(doc["name"], gc.Equals, name)
	c.Check(doc["env-uuid"], gc.Equals, envUUID)
	c.Check(doc["counter"], gc.Equals, value)
}

package mstate_test

import (
	. "launchpad.net/gocheck"
	state "launchpad.net/juju-core/juju/mstate"
	"launchpad.net/mgo"
	"launchpad.net/mgo/bson"
	"testing"
)

func Test(t *testing.T) { TestingT(t) }

var _ = Suite(&StateSuite{})

type StateSuite struct {
	MgoSuite
	st *state.State
}

func (s *StateSuite) SetUpTest(c *C) {
	s.MgoSuite.SetUpTest(c)
	st, err := state.Dial(mgoaddr)
	c.Assert(err, IsNil)
	s.st = st
}

func (s *StateSuite) TearDownTest(c *C) {
	s.st.Close()
	s.MgoSuite.TearDownTest(c)
}

func (s *StateSuite) assertMachineCount(c *C, expect int) {
	ms, err := s.st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, expect)
}

func (s *StateSuite) TestAllMachines(c *C) {
	session, err := mgo.Dial(mgoaddr)
	c.Assert(err, IsNil)
	defer session.Close()

	numInserts := 42
	mCollection := session.DB("juju").C("machines")
	ids := make([]bson.ObjectId, numInserts)
	for i := 0; i < numInserts; i++ {
		ids[i] = bson.NewObjectId()
		err := mCollection.Insert(bson.D{{"_id", ids[i]}})
		c.Assert(err, IsNil)
	}
	s.assertMachineCount(c, numInserts)
	ms, err := s.st.AllMachines()
	for k, v := range ms {
		c.Assert(v.Id(), Equals, ids[k].Hex())
	}
}

func (s *StateSuite) TestAddMachine(c *C) {
	m0, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	m, err := s.st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(m, HasLen, 1)
	c.Assert(m[0].Id(), Equals, m0.Id())

	numInserts := 42
	for i := 0; i < numInserts; i++ {
		_, err := s.st.AddMachine()
		c.Assert(err, IsNil)
	}
	s.assertMachineCount(c, 1+numInserts)
}

func (s *StateSuite) TestRemoveMachine(c *C) {
	m0, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	m1, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = s.st.RemoveMachine(m0.Id())
	c.Assert(err, IsNil)
	s.assertMachineCount(c, 1)
	ms, err := s.st.AllMachines()
	c.Assert(ms[0].Id(), Equals, m1.Id())
}

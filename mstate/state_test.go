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
	st       *state.State
	session  *mgo.Session
	machines *mgo.Collection
}

func (s *StateSuite) SetUpTest(c *C) {
	s.MgoSuite.SetUpTest(c)
	st, err := state.Dial(mgoaddr)
	c.Assert(err, IsNil)
	s.st = st
	session, err := mgo.Dial(mgoaddr)
	c.Assert(err, IsNil)
	s.session = session
	s.machines = session.DB("juju").C("machines")
}

func (s *StateSuite) TearDownTest(c *C) {
	s.st.Close()
	s.session.Close()
	s.MgoSuite.TearDownTest(c)
}

func (s *StateSuite) assertMachineCount(c *C, expect int) {
	ms, err := s.st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, expect)
}

func (s *StateSuite) TestAllMachines(c *C) {
	numInserts := 42
	ids := make([]bson.ObjectId, numInserts)
	for i := 0; i < numInserts; i++ {
		ids[i] = bson.NewObjectId()
		err := s.machines.Insert(bson.D{{"_id", ids[i]}})
		c.Assert(err, IsNil)
	}
	s.assertMachineCount(c, numInserts)
	ms, _ := s.st.AllMachines()
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

	// TODO: Removing a non-existing machine has to fail.
}

func (s *StateSuite) TestMachineInstanceId(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = s.machines.Update(bson.D{{"_id", bson.ObjectIdHex(machine.Id())}}, bson.D{{"instanceid", "spaceship/0"}})
	c.Assert(err, IsNil)

	iid, err := machine.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(iid, Equals, "spaceship/0")
}

func (s *StateSuite) TestMachineSetInstanceId(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = machine.SetInstanceId("umbrella/0")
	c.Assert(err, IsNil)

	n, err := s.machines.Find(bson.D{{"instanceid", "umbrella/0"}}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 1)
}

func (s *StateSuite) TestReadMachine(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	expectedId := machine.Id()
	machine, err = s.st.Machine(expectedId)
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Equals, expectedId)
}

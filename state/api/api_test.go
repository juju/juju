package api_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"net"
	stdtesting "testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type suite struct {
	testing.JujuConnSuite
	listener net.Listener
}

var _ = Suite(&suite{})

func (s *suite) TestMachineInstanceId(c *C) {
	stm, err := s.State.AddMachine(state.JobHostUnits)
	c.Assert(err, IsNil)
	m, err := s.APIState.Machine(stm.Id())
	c.Assert(err, IsNil)
	instId, err := m.InstanceId()
	c.Check(instId, Equals, "")
	c.Check(err, ErrorMatches, "instance id for machine 0 not found")

	err = stm.SetInstanceId("foo")
	c.Assert(err, IsNil)

	instId, err = m.InstanceId()
	c.Check(instId, Equals, "")
	c.Check(err, ErrorMatches, "instance id for machine 0 not found")

	err = m.Refresh()
	c.Assert(err, IsNil)

	instId, err = m.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(instId, Equals, "foo")
}

//func (s *suite) TestStop(c *C) {
//	stm, err := s.State.AddMachine(state.JobHostUnits)
//	c.Assert(err, IsNil)
//	err = stm.SetInstanceId("foo")
//	c.Assert(err, IsNil)
//
//	err = s.srv.Stop()
//	c.Assert(err, IsNil)
//	_, err = s.APIState.Machine(stm.Id())
//	c.Assert(err, ErrorMatches, "cannot receive response: EOF")
//
//	// Check it can be stopped twice.
//	err = s.srv.Stop()
//	c.Assert(err, IsNil)
//}

//func (s *suite) BenchmarkRequests(c *C) {
//	stm, err := s.State.AddMachine(state.JobHostUnits)
//	c.Assert(err, IsNil)
//	c.ResetTimer()
//	for i := 0; i < c.N; i++ {
//		_, err := s.APIState.Machine(stm.Id())
//		if err != nil {
//			c.Assert(err, IsNil)
//		}
//	}
//}
//
//func (s *suite) BenchmarkTestRequests(c *C) {
//	for i := 0; i < c.N; i++ {
//		err := s.APIState.TestRequest()
//		if err != nil {
//			c.Assert(err, IsNil)
//		}
//	}
//}

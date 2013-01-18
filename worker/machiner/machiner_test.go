package machiner_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/machiner"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type MachinerSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&MachinerSuite{})

func (s *MachinerSuite) TestNotFound(c *C) {
	mr := machiner.NewMachiner(s.State, "eleventy-one")
	c.Assert(mr.Wait(), Equals, worker.ErrDead)
}

func (s *MachinerSuite) TestRunStop(c *C) {
	m, err := s.State.AddMachine(state.JobHostUnits)
	c.Assert(err, IsNil)
	mr := machiner.NewMachiner(s.State, m.Id())
	c.Assert(mr.Stop(), IsNil)
	c.Assert(m.Refresh(), IsNil)
	c.Assert(m.Life(), Equals, state.Alive)
}

func (s *MachinerSuite) TestSetDead(c *C) {
	m, err := s.State.AddMachine(state.JobHostUnits)
	c.Assert(err, IsNil)
	mr := machiner.NewMachiner(s.State, m.Id())
	defer mr.Stop()
	c.Assert(m.Destroy(), IsNil)
	s.State.StartSync()
	c.Assert(mr.Wait(), Equals, worker.ErrDead)
	c.Assert(m.Refresh(), IsNil)
	c.Assert(m.Life(), Equals, state.Dead)
}

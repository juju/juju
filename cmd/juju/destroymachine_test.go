package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type DestroyMachineSuite struct {
	repoSuite
}

var _ = Suite(&DestroyMachineSuite{})

func runDestroyMachine(c *C, args ...string) error {
	com := &DestroyMachineCommand{}
	if err := com.Init(newFlagSet(), args); err != nil {
		return err
	}
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *DestroyMachineSuite) TestDestroyMachine(c *C) {
	// Create a machine running a unit.
	testing.Charms.BundlePath(s.seriesPath, "series", "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)

	// Get the state entities to allow sane testing.
	u, err := s.State.Unit("riak/0")
	c.Assert(err, IsNil)
	mid, err := u.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(mid, Equals, "0")

	// Try to destroy the machine and fail.
	err = runDestroyMachine(c, "0")
	c.Assert(err, ErrorMatches, `machine 0 cannot become dying: unit "riak/0" is assigned to it`)

	// Remove the unit, and try to destroy the machine along with another that
	// doesn't exist; check nothing's removed.
	err = u.Destroy()
	c.Assert(err, IsNil)
	err = u.EnsureDead()
	c.Assert(err, IsNil)
	err = u.Remove()
	c.Assert(err, IsNil)
	err = runDestroyMachine(c, "0", "1")
	c.Assert(err, ErrorMatches, `machine 1 not found`)
	m, err := s.State.Machine("0")
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, state.Alive)

	// Just destroy the machine.
	err = runDestroyMachine(c, "0")
	c.Assert(err, IsNil)
	err = m.Refresh()
	c.Assert(err, IsNil)
	c.Assert(m.Life(), Equals, state.Dying)

	// Check invalid args.
	err = runDestroyMachine(c)
	c.Assert(err, ErrorMatches, `no machines specified`)
	err = runDestroyMachine(c, "1", "2", "nonsense", "rubbish")
	c.Assert(err, ErrorMatches, `invalid machine id "nonsense"`)
}

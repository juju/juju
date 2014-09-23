// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/firewaller"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type stateSuite struct {
	firewallerSuite
	*apitesting.EnvironWatcherTests
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)
	s.EnvironWatcherTests = apitesting.NewEnvironWatcherTests(s.firewaller, s.BackingState, true)
}

func (s *stateSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *stateSuite) TestWatchEnvironMachines(c *gc.C) {
	w, err := s.firewaller.WatchEnvironMachines()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange(s.machines[0].Id(), s.machines[1].Id(), s.machines[2].Id())

	// Add another machine make sure they are detected.
	otherMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	wc.AssertChange(otherMachine.Id())

	// Change the life cycle of last machine.
	err = otherMachine.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertChange(otherMachine.Id())

	// Add a container and make sure it's not detected.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err = s.State.AddMachineInsideMachine(template, s.machines[0].Id(), instance.LXC)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *stateSuite) TestWatchOpenedPortsNotImplementedV0(c *gc.C) {
	s.patchNewState(c, firewaller.NewStateV0)

	w, err := s.firewaller.WatchOpenedPorts()
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
	c.Assert(err, gc.ErrorMatches, `WatchOpenedPorts\(\) \(need V1\+\) not implemented`)
	c.Assert(w, gc.IsNil)
}

func (s *stateSuite) TestWatchOpenedPortsV1(c *gc.C) {
	s.patchNewState(c, firewaller.NewStateV1)

	// Open some ports.
	err := s.units[0].OpenPort("tcp", 1234)
	c.Assert(err, gc.IsNil)
	err = s.units[2].OpenPort("udp", 4321)
	c.Assert(err, gc.IsNil)

	w, err := s.firewaller.WatchOpenedPorts()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)

	expectChanges := []string{
		"0:juju-public",
		"2:juju-public",
	}
	wc.AssertChangeInSingleEvent(expectChanges...)
	wc.AssertNoChange()

	// Close a port, make sure it's detected.
	err = s.units[2].ClosePort("udp", 4321)
	c.Assert(err, gc.IsNil)

	wc.AssertChange(expectChanges[1])
	wc.AssertNoChange()

	// Close it again, no changes.
	err = s.units[2].ClosePort("udp", 4321)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Open existing port, no changes.
	err = s.units[0].ClosePort("udp", 1234)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Open another port, ensure it's detected.
	err = s.units[1].OpenPort("tcp", 8080)
	c.Assert(err, gc.IsNil)
	wc.AssertChange("1:juju-public")
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

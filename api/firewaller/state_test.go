// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"strings"
	"time"

	gc "launchpad.net/gocheck"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
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

func (s *stateSuite) TestWatchOpenedPorts(c *gc.C) {
	watcher, err := s.firewaller.WatchOpenedPorts()
	c.Assert(err, gc.IsNil)
	changes := watcher.Changes()

	var change []string
	select {
	case change = <-changes:
		c.Assert(change, gc.HasLen, 0)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out while waiting for port watcher")
	}

	factory := factory.NewFactory(s.State)
	unit := factory.MakeUnit(c, nil)
	err = unit.OpenPort("tcp", 80)
	c.Assert(err, gc.IsNil)

	select {
	case change = <-changes:
		c.Assert(change, gc.HasLen, 1)
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out while waiting for port watcher")
	}

	changeComponents := strings.Split(change[0], ":")
	c.Assert(changeComponents, gc.HasLen, 2)

	assignedMachineId, err := unit.AssignedMachineId()
	c.Assert(err, gc.IsNil)
	c.Assert(changeComponents[0], gc.Equals, assignedMachineId)
	c.Assert(changeComponents[1], gc.Equals, network.DefaultPublic)

	c.Assert(watcher.Stop(), gc.IsNil)
}

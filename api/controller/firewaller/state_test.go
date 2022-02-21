// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/state"
)

type stateSuite struct {
	firewallerSuite
	*apitesting.ModelWatcherTests
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)
	s.ModelWatcherTests = apitesting.NewModelWatcherTests(s.firewaller, s.BackingState, s.Model)
}

func (s *stateSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *stateSuite) TestWatchModelMachines(c *gc.C) {
	w, err := s.firewaller.WatchModelMachines()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange(s.machines[0].Id(), s.machines[1].Id(), s.machines[2].Id())

	// Add another machine make sure they are detected.
	otherMachine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(otherMachine.Id())

	// Change the life cycle of last machine.
	err = otherMachine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(otherMachine.Id())

	// Add a container and make sure it's not detected.
	template := state.MachineTemplate{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	}
	_, err = s.State.AddMachineInsideMachine(template, s.machines[0].Id(), instance.LXD)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *stateSuite) TestWatchOpenedPorts(c *gc.C) {
	// Open some ports.
	mustOpenPortRanges(c, s.State, s.units[0], allEndpoints, []network.PortRange{
		network.MustParsePortRange("1234-1400/tcp"),
	})
	mustOpenPortRanges(c, s.State, s.units[2], allEndpoints, []network.PortRange{
		network.MustParsePortRange("4321/udp"),
	})

	w, err := s.firewaller.WatchOpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	expectChanges := []string{
		"0",
		"2",
	}
	wc.AssertChangeInSingleEvent(expectChanges...)
	wc.AssertNoChange()

	// Close a port, make sure it's detected.
	mustClosePortRanges(c, s.State, s.units[2], allEndpoints, []network.PortRange{
		network.MustParsePortRange("4321/udp"),
	})

	wc.AssertChange(expectChanges[1])
	wc.AssertNoChange()

	// Close it again, no changes.
	mustClosePortRanges(c, s.State, s.units[2], allEndpoints, []network.PortRange{
		network.MustParsePortRange("4321/udp"),
	})
	wc.AssertNoChange()

	// Close non-existing port, no changes.
	mustClosePortRanges(c, s.State, s.units[2], allEndpoints, []network.PortRange{
		network.MustParsePortRange("1234/udp"),
	})
	wc.AssertNoChange()

	// Open another port range, ensure it's detected.
	mustOpenPortRanges(c, s.State, s.units[1], allEndpoints, []network.PortRange{
		network.MustParsePortRange("8080-8088/udp"),
	})
	wc.AssertChange("1")
	wc.AssertNoChange()
}

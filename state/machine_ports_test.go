// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type MachinePortsDocSuite struct {
	ConnSuite
	charm          *state.Charm
	application    *state.Application
	unit1          *state.Unit
	unit2          *state.Unit
	machine        *state.Machine
	subnet         *state.Subnet
	machPortRanges state.MachinePortRanges
}

var _ = gc.Suite(&MachinePortsDocSuite{})

const allEndpoints = ""

func (s *MachinePortsDocSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	s.charm = s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: s.charm})
	s.machine = s.Factory.MakeMachine(c, &factory.MachineParams{Base: state.UbuntuBase("12.10")})
	s.unit1 = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application, Machine: s.machine})
	s.unit2 = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application, Machine: s.machine})

	var err error
	s.subnet, err = s.State.AddSubnet(network.SubnetInfo{CIDR: "0.1.2.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	s.machPortRanges, err = s.machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machPortRanges.UniquePortRanges(), gc.HasLen, 0, gc.Commentf("expected no port ranges to be open for machine"))
}

func assertRefreshMachinePortsDoc(c *gc.C, p state.MachinePortRanges, errIs error) {
	type refresher interface {
		Refresh() error
	}

	portRefresher, supports := p.(refresher)
	c.Assert(supports, jc.IsTrue, gc.Commentf("machine ports interface does not implement Refresh()"))

	err := portRefresher.Refresh()
	if errIs != nil {
		c.Assert(err, jc.ErrorIs, errIs)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func assertRemoveMachinePortsDoc(c *gc.C, p state.MachinePortRanges) {
	type remover interface {
		Remove() error
	}

	portRemover, supports := p.(remover)
	c.Assert(supports, jc.IsTrue, gc.Commentf("port document does not implement Remove()"))
	c.Assert(portRemover.Remove(), jc.ErrorIsNil)
}

func assertMachinePortsPersisted(c *gc.C, p state.MachinePortRanges, persisted bool) {
	type persistChecker interface {
		Persisted() bool
	}

	checker, supports := p.(persistChecker)
	c.Assert(supports, jc.IsTrue, gc.Commentf("machine ports interface does not implement Persisted()"))
	c.Assert(checker.Persisted(), gc.Equals, persisted)
}

func (s *MachinePortsDocSuite) mustOpenCloseMachinePorts(c *gc.C, machPorts state.MachinePortRanges, unitName, endpointName string, openRanges, closeRanges []network.PortRange) {
	err := s.openCloseMachinePorts(machPorts, unitName, endpointName, openRanges, closeRanges)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachinePortsDocSuite) openCloseMachinePorts(machPorts state.MachinePortRanges, unitName, endpointName string, openRanges, closeRanges []network.PortRange) error {
	unitPorts := machPorts.ForUnit(unitName)
	for _, pr := range openRanges {
		unitPorts.Open(endpointName, pr)
	}
	for _, pr := range closeRanges {
		unitPorts.Close(endpointName, pr)
	}
	return s.State.ApplyOperation(machPorts.Changes())
}

func (s *MachinePortsDocSuite) TestModelAllOpenPortRanges(c *gc.C) {
	toOpen := []network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
		network.MustParsePortRange("300-400/tcp"),
		network.MustParsePortRange("500-600/tcp"),
	}
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, toOpen[0:1], nil)
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit2.Name(), allEndpoints, toOpen[1:2], nil)

	// Add a second machine with another unit and open the last port range
	mach2 := s.Factory.MakeMachine(c, &factory.MachineParams{Base: state.UbuntuBase("12.10")})
	unit3 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application, Machine: mach2})
	mach2Ports, err := mach2.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	s.mustOpenCloseMachinePorts(c, mach2Ports, unit3.Name(), allEndpoints, toOpen[2:3], nil)

	allMachinePortRanges, err := s.Model.OpenedPortRangesForAllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allMachinePortRanges, gc.HasLen, 2)

	c.Assert(allMachinePortRanges[0].UniquePortRanges(), gc.DeepEquals, toOpen[0:2])
	c.Assert(allMachinePortRanges[1].UniquePortRanges(), gc.DeepEquals, toOpen[2:])
}

func (s *MachinePortsDocSuite) TestOpenMachinePortsForWildcardEndpoint(c *gc.C) {
	s.testOpenPortsForEndpoint(c, allEndpoints)
}

func (s *MachinePortsDocSuite) TestOpenMachinePortsForEndpoint(c *gc.C) {
	s.testOpenPortsForEndpoint(c, "monitoring-port")
}

func (s *MachinePortsDocSuite) testOpenPortsForEndpoint(c *gc.C, endpoint string) {
	toOpen := []network.PortRange{network.MustParsePortRange("100-200/tcp")}
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), endpoint, toOpen, nil)

	machPorts, err := s.machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machPorts.MachineID(), gc.Equals, s.machine.Id())
	c.Assert(machPorts.UniquePortRanges(), gc.DeepEquals, toOpen)
}

func (s *MachinePortsDocSuite) TestOpenAndCloseMachinePorts(c *gc.C) {
	type unitPortRange struct {
		UnitName  string
		PortRange network.PortRange
	}
	testCases := []struct {
		about    string
		existing []unitPortRange
		toOpen   *unitPortRange
		toClose  *unitPortRange
		expected string
	}{{
		about:    "open and close same port range",
		existing: nil,
		toOpen: &unitPortRange{
			PortRange: network.MustParsePortRange("100-200/tcp"),
			UnitName:  s.unit1.Name(),
		},
		toClose: &unitPortRange{
			PortRange: network.MustParsePortRange("100-200/tcp"),
			UnitName:  s.unit1.Name(),
		},
		expected: "",
	}, {
		about:    "open and close icmp",
		existing: nil,
		toOpen: &unitPortRange{
			PortRange: network.MustParsePortRange("icmp"),
			UnitName:  s.unit1.Name(),
		},
		toClose: &unitPortRange{
			PortRange: network.MustParsePortRange("icmp"),
			UnitName:  s.unit1.Name(),
		},
		expected: "",
	}, {
		about: "try to close part of a port range",
		existing: []unitPortRange{{
			PortRange: network.MustParsePortRange("100-200/tcp"),
			UnitName:  s.unit1.Name(),
		}},
		toOpen: nil,
		toClose: &unitPortRange{
			PortRange: network.MustParsePortRange("100-150/tcp"),
			UnitName:  s.unit1.Name(),
		},
		expected: `cannot close ports 100-150/tcp: port ranges 100-200/tcp \("wordpress/0"\) and 100-150/tcp \("wordpress/0"\) conflict`,
	}, {
		about: "close a port range opened by another unit",
		existing: []unitPortRange{{
			PortRange: network.MustParsePortRange("100-150/tcp"),
			UnitName:  s.unit2.Name(),
		}},
		toOpen: nil,
		toClose: &unitPortRange{
			PortRange: network.MustParsePortRange("100-150/tcp"),
			UnitName:  s.unit1.Name(),
		},
		expected: `cannot close ports 100-150/tcp: port ranges 100-150/tcp \("wordpress/1"\) and 100-150/tcp \("wordpress/0"\) conflict`,
	}, {
		about: "open twice the same port range",
		existing: []unitPortRange{{
			PortRange: network.MustParsePortRange("100-150/tcp"),
			UnitName:  s.unit1.Name(),
		}},
		toOpen: &unitPortRange{
			PortRange: network.MustParsePortRange("100-150/tcp"),
			UnitName:  s.unit1.Name(),
		},
		toClose:  nil,
		expected: "",
	}, {
		about:    "close an unopened port range",
		existing: nil,
		toOpen:   nil,
		toClose: &unitPortRange{
			PortRange: network.MustParsePortRange("100-150/tcp"),
			UnitName:  s.unit1.Name(),
		},
		expected: "",
	}, {
		about: "try to close an overlapping port range",
		existing: []unitPortRange{{
			PortRange: network.MustParsePortRange("100-200/tcp"),
			UnitName:  s.unit1.Name(),
		}},
		toOpen: nil,
		toClose: &unitPortRange{
			PortRange: network.MustParsePortRange("100-300/tcp"),
			UnitName:  s.unit1.Name(),
		},
		expected: `cannot close ports 100-300/tcp: port ranges 100-200/tcp \("wordpress/0"\) and 100-300/tcp \("wordpress/0"\) conflict`,
	}, {
		about: "try to open an overlapping port range with different unit",
		existing: []unitPortRange{{
			PortRange: network.MustParsePortRange("100-200/tcp"),
			UnitName:  s.unit1.Name(),
		}},
		toOpen: &unitPortRange{
			PortRange: network.MustParsePortRange("100-300/tcp"),
			UnitName:  s.unit2.Name(),
		},
		expected: `cannot open ports 100-300/tcp: port ranges 100-200/tcp \("wordpress/0"\) and 100-300/tcp \("wordpress/1"\) conflict`,
	}, {
		about: "try to open an identical port range with different unit",
		existing: []unitPortRange{{
			PortRange: network.MustParsePortRange("100-200/tcp"),
			UnitName:  s.unit1.Name(),
		}},
		toOpen: &unitPortRange{
			PortRange: network.MustParsePortRange("100-200/tcp"),
			UnitName:  s.unit2.Name(),
		},
		expected: `cannot open ports 100-200/tcp: port ranges 100-200/tcp \("wordpress/0"\) and 100-200/tcp \("wordpress/1"\) conflict`,
	}, {
		about: "try to open a port range with different protocol with different unit",
		existing: []unitPortRange{{
			PortRange: network.MustParsePortRange("100-200/tcp"),
			UnitName:  s.unit1.Name(),
		}},
		toOpen: &unitPortRange{
			PortRange: network.MustParsePortRange("100-200/udp"),
			UnitName:  s.unit2.Name(),
		},
		expected: "",
	}, {
		about: "try to open a non-overlapping port range with different unit",
		existing: []unitPortRange{{
			PortRange: network.MustParsePortRange("100-200/tcp"), UnitName: s.unit1.Name(),
		}},
		toOpen: &unitPortRange{
			PortRange: network.MustParsePortRange("300-400/tcp"),
			UnitName:  s.unit2.Name(),
		},
		expected: "",
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)

		ports, err := s.machine.OpenedPortRanges()
		c.Assert(err, jc.ErrorIsNil)

		// open ports that should exist for the test case
		for _, pr := range t.existing {
			s.mustOpenCloseMachinePorts(c, ports, pr.UnitName, allEndpoints, []network.PortRange{pr.PortRange}, nil)
		}
		if len(t.existing) != 0 {
			assertRefreshMachinePortsDoc(c, ports, nil)
		}
		if t.toOpen != nil {
			err := s.openCloseMachinePorts(ports, t.toOpen.UnitName, allEndpoints, []network.PortRange{t.toOpen.PortRange}, nil)
			if t.expected == "" {
				c.Check(err, jc.ErrorIsNil)
			} else {
				c.Check(err, gc.ErrorMatches, t.expected)
			}
			assertRefreshMachinePortsDoc(c, ports, nil)
		}

		if t.toClose != nil {
			err := s.openCloseMachinePorts(ports, t.toClose.UnitName, allEndpoints, nil, []network.PortRange{t.toClose.PortRange})
			if t.expected == "" {
				c.Check(err, jc.ErrorIsNil)
			} else {
				c.Check(err, gc.ErrorMatches, t.expected)
			}
		}
		assertRemoveMachinePortsDoc(c, ports)
	}
}

func (s *MachinePortsDocSuite) TestClosePortRangeOperationSucceedsForDyingModel(c *gc.C) {
	// Open initial port range
	toOpen := []network.PortRange{network.MustParsePortRange("200-210/tcp")}
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, toOpen, nil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.Model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	}).Check()

	// Close the initially opened ports
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints,
		nil,
		[]network.PortRange{network.MustParsePortRange("200-210/tcp")},
	)
}

func (s *MachinePortsDocSuite) TestComposedOpenCloseOperation(c *gc.C) {
	// Open initial port range
	toOpen := []network.PortRange{network.MustParsePortRange("200-210/tcp")}
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, toOpen, nil)

	// Run a composed open/close operation
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints,
		[]network.PortRange{network.MustParsePortRange("400-500/tcp")},
		[]network.PortRange{network.MustParsePortRange("200-210/tcp")},
	)

	// Enumerate ports
	assertRefreshMachinePortsDoc(c, s.machPortRanges, nil)
	unitRanges := s.machPortRanges.ForUnit(s.unit1.Name()).ForEndpoint(allEndpoints)
	c.Assert(unitRanges, gc.DeepEquals, []network.PortRange{network.MustParsePortRange("400-500/tcp")})

	// If we open and close the same set of ports the port doc should be deleted.
	assertRefreshMachinePortsDoc(c, s.machPortRanges, nil)
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints,
		[]network.PortRange{network.MustParsePortRange("400-500/tcp")},
		[]network.PortRange{network.MustParsePortRange("400-500/tcp")},
	)

	// The next refresh should fail with ErrNotFound as the document has been removed.
	assertRefreshMachinePortsDoc(c, s.machPortRanges, errors.NotFound)
}

func (s *MachinePortsDocSuite) TestComposedOpenCloseOperationNoEffectiveOps(c *gc.C) {
	// Run a composed open/close operation
	unitPortRanges := s.machPortRanges.ForUnit(s.unit1.Name())

	// Duplicate range should be skipped
	unitPortRanges.Open("monitoring-port", network.MustParsePortRange("400-500/tcp"))
	unitPortRanges.Open("monitoring-port", network.MustParsePortRange("400-500/tcp"))

	unitPortRanges.Close("monitoring-port", network.MustParsePortRange("400-500/tcp"))

	// As the doc does not exist and the end result is still an empty port range
	// this should return ErrNoOperations
	_, err := s.machPortRanges.Changes().Build(0)
	c.Assert(err, gc.Equals, jujutxn.ErrNoOperations)
}

func (s *MachinePortsDocSuite) TestICMP(c *gc.C) {
	// Open initial port range
	toOpen := []network.PortRange{network.MustParsePortRange("icmp")}
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, toOpen, nil)

	ranges := s.machPortRanges.ForUnit(s.unit1.Name()).UniquePortRanges()
	c.Assert(ranges, gc.HasLen, 1)
}

func (s *MachinePortsDocSuite) TestOpenInvalidRange(c *gc.C) {
	toOpen := []network.PortRange{{FromPort: 400, ToPort: 200, Protocol: "tcp"}}
	err := s.openCloseMachinePorts(s.machPortRanges, s.unit1.Name(), allEndpoints, toOpen, nil)
	c.Assert(err, gc.ErrorMatches, `cannot open ports 400-200/tcp: invalid port range 400-200/tcp`)
}

func (s *MachinePortsDocSuite) TestCloseInvalidRange(c *gc.C) {
	// Open initial port range
	toOpen := []network.PortRange{network.MustParsePortRange("100-200/tcp")}
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, toOpen, nil)

	assertRefreshMachinePortsDoc(c, s.machPortRanges, nil)

	toClose := []network.PortRange{{FromPort: 150, ToPort: 200, Protocol: "tcp"}}
	err := s.openCloseMachinePorts(s.machPortRanges, s.unit1.Name(), allEndpoints, nil, toClose)
	c.Assert(err, gc.ErrorMatches, `cannot close ports 150-200/tcp: port ranges 100-200/tcp \("wordpress/0"\) and 150-200/tcp \("wordpress/0"\) conflict`)
}

func (s *MachinePortsDocSuite) TestRemoveMachinePortsDoc(c *gc.C) {
	// Open initial port range
	toOpen := []network.PortRange{network.MustParsePortRange("100-200/tcp")}
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, toOpen, nil)

	// Remove document
	assertRemoveMachinePortsDoc(c, s.machPortRanges)

	// If we lookup the opened ports for the machine we should now get a blank doc.
	machPorts, err := s.machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	assertMachinePortsPersisted(c, machPorts, false)
}

func (s *MachinePortsDocSuite) TestWatchMachinePorts(c *gc.C) {
	// No port ranges open initially, no changes.
	w := s.State.WatchOpenedPorts()
	c.Assert(w, gc.NotNil)

	defer workertest.CleanKill(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)
	// The first change we get is an empty one, as there are no ports
	// opened yet and we need an initial event for the API watcher to
	// work.
	wc.AssertChange()
	wc.AssertNoChange()

	portRange := network.MustParsePortRange("100-200/tcp")

	// Open a port range, detect a change.
	expectChange := s.machine.Id()
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, []network.PortRange{portRange}, nil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Close the port range, detect a change.
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, nil, []network.PortRange{portRange})
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Close the port range again, no changes.
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, nil, []network.PortRange{portRange})
	wc.AssertNoChange()

	// Open another range, detect a change.
	portRange = network.MustParsePortRange("999-1999/udp")
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), "monitoring-port", []network.PortRange{portRange}, nil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Open the same range again, no changes.
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), "monitoring-port", []network.PortRange{portRange}, nil)
	wc.AssertNoChange()

	// Open another range, detect a change.
	otherRange := network.MustParsePortRange("1-100/tcp")
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, []network.PortRange{otherRange}, nil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Close the other range, detect a change.
	s.mustOpenCloseMachinePorts(c, s.machPortRanges, s.unit1.Name(), allEndpoints, nil, []network.PortRange{otherRange})
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Remove the ports document, detect a change.
	assertRemoveMachinePortsDoc(c, s.machPortRanges)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// And again - no change.
	assertRemoveMachinePortsDoc(c, s.machPortRanges)
	wc.AssertNoChange()
}

func (s *MachinePortsDocSuite) TestChangesForIndividualUnits(c *gc.C) {
	unit1PortRanges := s.machPortRanges.ForUnit(s.unit1.Name())
	unit1PortRanges.Open(allEndpoints, network.MustParsePortRange("100-200/tcp"))

	unit2PortRanges := s.machPortRanges.ForUnit(s.unit2.Name())
	unit2PortRanges.Open(allEndpoints, network.MustParsePortRange("8080/tcp"))

	// Apply changes scoped to unit 1. The recorded changes for unit 2
	// in the machine port ranges instance should remain intact.
	c.Assert(s.State.ApplyOperation(unit1PortRanges.Changes()), jc.ErrorIsNil)

	// Check that the existing machine port ranges instance reflects the
	// unit 1 changes we just applied.
	c.Assert(s.machPortRanges.UniquePortRanges(), gc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
	}, gc.Commentf("machine port ranges instance not updated correctly after unit-scoped port change application"))

	// Grab a fresh copy of the machine ranges and verify the expected ports
	// have been correctly persisted.
	freshMachPortRanges, err := s.machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(freshMachPortRanges.UniquePortRanges(), gc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
	}, gc.Commentf("unit 1 changes were not correctly persisted to DB"))

	// Apply pending changes scoped to unit 2.
	c.Assert(s.State.ApplyOperation(unit2PortRanges.Changes()), jc.ErrorIsNil)

	// Check that the existing machine port ranges instance reflects both
	// unit 1 and unit 2 changes
	c.Assert(s.machPortRanges.UniquePortRanges(), gc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
		network.MustParsePortRange("8080/tcp"),
	}, gc.Commentf("machine port ranges instance not updated correctly after unit-scoped port change application"))

	// Grab a fresh copy of the machine ranges and verify the expected ports
	// have been correctly persisted.
	freshMachPortRanges, err = s.machine.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(freshMachPortRanges.UniquePortRanges(), gc.DeepEquals, []network.PortRange{
		network.MustParsePortRange("100-200/tcp"),
		network.MustParsePortRange("8080/tcp"),
	}, gc.Commentf("unit changes were not correctly persisted to DB"))

	// Verify that if we call changes on the machine ports instance we
	// get no ops as everything has been committed.
	_, err = s.machPortRanges.Changes().Build(0)
	c.Assert(err, gc.Equals, jujutxn.ErrNoOperations, gc.Commentf("machine port range was not synced correctly after applying changes"))
}

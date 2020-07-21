// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type MachinePortsDocSuite struct {
	ConnSuite
	charm              *state.Charm
	application        *state.Application
	unit1              *state.Unit
	unit2              *state.Unit
	machine            *state.Machine
	subnet             *state.Subnet
	portsInSubnet      state.MachinePortRanges
	portsWithoutSubnet state.MachinePortRanges
}

var _ = gc.Suite(&MachinePortsDocSuite{})

func (s *MachinePortsDocSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	s.charm = s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: s.charm})
	s.machine = s.Factory.MakeMachine(c, &factory.MachineParams{Series: "quantal"})
	s.unit1 = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application, Machine: s.machine})
	s.unit2 = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application, Machine: s.machine})

	var err error
	s.subnet, err = s.State.AddSubnet(network.SubnetInfo{CIDR: "0.1.2.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	s.portsInSubnet, err = s.machine.OpenedPortsInSubnet(s.subnet.ID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.portsInSubnet, gc.NotNil)

	s.portsWithoutSubnet, err = s.machine.OpenedPortsInSubnet("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.portsWithoutSubnet, gc.NotNil)
}

func assertRefreshMachinePortsDoc(c *gc.C, p state.MachinePortRanges, errSatisfier func(error) bool) {
	type refresher interface {
		Refresh() error
	}

	portRefresher, supports := p.(refresher)
	c.Assert(supports, jc.IsTrue, gc.Commentf("machine ports interface does not implement Refresh()"))

	err := portRefresher.Refresh()
	if errSatisfier != nil {
		c.Assert(err, jc.Satisfies, errSatisfier)
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

func (s *MachinePortsDocSuite) mustOpenCloseMachinePorts(c *gc.C, ports state.MachinePortRanges, unitName string, openRange, closeRange []network.PortRange) {
	c.Assert(s.openCloseMachinePorts(ports, unitName, openRange, closeRange), jc.ErrorIsNil)
}

func (s *MachinePortsDocSuite) openCloseMachinePorts(ports state.MachinePortRanges, unitName string, openRange, closeRange []network.PortRange) error {
	op, err := ports.OpenClosePortsOperation(unitName, openRange, closeRange)
	if err != nil {
		return err
	}
	return s.State.ApplyOperation(op)
}

func (s *MachinePortsDocSuite) TestModelAllMachinePorts(c *gc.C) {
	toOpen := []network.PortRange{network.MustParsePortRange("100-200/tcp")}
	s.mustOpenCloseMachinePorts(c, s.portsInSubnet, s.unit1.Name(), toOpen, nil)
	s.mustOpenCloseMachinePorts(c, s.portsWithoutSubnet, s.unit1.Name(), toOpen, nil)

	machine := s.Factory.MakeMachine(c, &factory.MachineParams{Series: "quantal"})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application, Machine: machine})
	ports, err := machine.OpenedPortsInSubnet(s.subnet.ID())
	c.Assert(err, jc.ErrorIsNil)
	s.mustOpenCloseMachinePorts(c, ports, unit.Name(), toOpen, nil)

	ports, err = machine.OpenedPortsInSubnet("")
	c.Assert(err, jc.ErrorIsNil)
	s.mustOpenCloseMachinePorts(c, ports, unit.Name(), toOpen, nil)

	allMachinePorts, err := s.Model.OpenedPortsForAllMachines()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allMachinePorts, gc.HasLen, 4)
}

func (s *MachinePortsDocSuite) TestCreateMachinePortsWithSubnet(c *gc.C) {
	s.testCreateMachinePortsWithSubnetID(c, s.subnet.ID())
}

func (s *MachinePortsDocSuite) testCreateMachinePortsWithSubnetID(c *gc.C, subnetID string) {
	toOpen := []network.PortRange{network.MustParsePortRange("100-200/tcp")}
	ports, err := s.machine.OpenedPortsInSubnet(subnetID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.NotNil)
	s.mustOpenCloseMachinePorts(c, ports, s.unit1.Name(), toOpen, nil)
	c.Assert(err, jc.ErrorIsNil)

	ports, err = s.machine.OpenedPortsInSubnet(subnetID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports.MachineID(), gc.Equals, s.machine.Id())
	c.Assert(ports.SubnetID(), gc.Equals, subnetID)

	c.Assert(ports.PortRangesForUnit(s.unit1.Name()), gc.HasLen, 1)
}

func (s *MachinePortsDocSuite) TestCreateMachinePortsWithoutSubnet(c *gc.C) {
	s.testCreateMachinePortsWithSubnetID(c, "")
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
		about: "close an unopened port range with existing clash from other unit",
		existing: []unitPortRange{{
			PortRange: network.MustParsePortRange("100-150/tcp"),
			UnitName:  s.unit2.Name(),
		}},
		toOpen: nil,
		toClose: &unitPortRange{
			PortRange: network.MustParsePortRange("100-150/tcp"),
			UnitName:  s.unit1.Name(),
		},
		expected: "",
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

		ports, err := s.machine.OpenedPortsInSubnet(s.subnet.ID())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ports, gc.NotNil)

		// open ports that should exist for the test case
		for _, pr := range t.existing {
			s.mustOpenCloseMachinePorts(c, ports, pr.UnitName, []network.PortRange{pr.PortRange}, nil)
		}
		if len(t.existing) != 0 {
			assertRefreshMachinePortsDoc(c, ports, nil)
		}
		if t.toOpen != nil {
			err := s.openCloseMachinePorts(ports, t.toOpen.UnitName, []network.PortRange{t.toOpen.PortRange}, nil)
			if t.expected == "" {
				c.Check(err, jc.ErrorIsNil)
			} else {
				c.Check(err, gc.ErrorMatches, t.expected)
			}
			assertRefreshMachinePortsDoc(c, ports, nil)
		}

		if t.toClose != nil {
			err := s.openCloseMachinePorts(ports, t.toClose.UnitName, nil, []network.PortRange{t.toClose.PortRange})
			if t.expected == "" {
				c.Check(err, jc.ErrorIsNil)
			} else {
				c.Check(err, gc.ErrorMatches, t.expected)
			}
		}
		assertRemoveMachinePortsDoc(c, ports)
	}
}

func (s *MachinePortsDocSuite) TestComposedOpenCloseOperation(c *gc.C) {
	// Open initial port range
	toOpen := []network.PortRange{network.MustParsePortRange("200-210/tcp")}
	s.mustOpenCloseMachinePorts(c, s.portsWithoutSubnet, s.unit1.Name(), toOpen, nil)

	// Run a composed open/close operation
	s.mustOpenCloseMachinePorts(c, s.portsWithoutSubnet, s.unit1.Name(),
		[]network.PortRange{network.MustParsePortRange("400-500/tcp")},
		[]network.PortRange{network.MustParsePortRange("200-210/tcp")},
	)

	// Enumerate ports
	assertRefreshMachinePortsDoc(c, s.portsWithoutSubnet, nil)
	unitRanges := s.portsWithoutSubnet.PortRangesForUnit(s.unit1.Name())
	c.Assert(unitRanges, gc.DeepEquals, []network.PortRange{network.MustParsePortRange("400-500/tcp")})

	// If we open and close the same set of ports the port doc should be deleted.
	assertRefreshMachinePortsDoc(c, s.portsWithoutSubnet, nil)
	s.mustOpenCloseMachinePorts(c, s.portsWithoutSubnet, s.unit1.Name(),
		[]network.PortRange{network.MustParsePortRange("400-500/tcp")},
		[]network.PortRange{network.MustParsePortRange("400-500/tcp")},
	)

	// The next refresh should fail with ErrNotFound as the document has been removed.
	assertRefreshMachinePortsDoc(c, s.portsWithoutSubnet, errors.IsNotFound)
}

func (s *MachinePortsDocSuite) TestPortOperationSucceedsForDyingModel(c *gc.C) {
	// Open initial port range
	toOpen := []network.PortRange{network.MustParsePortRange("200-210/tcp")}
	s.mustOpenCloseMachinePorts(c, s.portsWithoutSubnet, s.unit1.Name(), toOpen, nil)

	defer state.SetBeforeHooks(c, s.State, func() {
		c.Assert(s.Model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	}).Check()

	// Close the initially opened ports
	s.mustOpenCloseMachinePorts(c, s.portsWithoutSubnet, s.unit1.Name(),
		nil,
		[]network.PortRange{network.MustParsePortRange("200-210/tcp")},
	)
}

func (s *MachinePortsDocSuite) TestComposedOpenCloseOperationNoEffectiveOps(c *gc.C) {
	// Run a composed open/close operation
	op, err := s.portsWithoutSubnet.OpenClosePortsOperation(s.unit1.Name(),
		// Open 400-500
		[]network.PortRange{
			network.MustParsePortRange("400-500/tcp"),
			// Duplicate range should be skipped
			network.MustParsePortRange("400-500/tcp"),
		},
		// Close 400-500
		[]network.PortRange{
			network.MustParsePortRange("400-500/tcp"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	// As the doc does not exist and the end result is still an empty port range
	// this should return ErrNoOperations
	_, err = op.Build(0)
	c.Assert(err, gc.Equals, jujutxn.ErrNoOperations)
}

func (s *MachinePortsDocSuite) TestICMP(c *gc.C) {
	// Open initial port range
	toOpen := []network.PortRange{network.MustParsePortRange("icmp")}
	s.mustOpenCloseMachinePorts(c, s.portsWithoutSubnet, s.unit1.Name(), toOpen, nil)

	ranges := s.portsWithoutSubnet.PortRangesForUnit(s.unit1.Name())
	c.Assert(ranges, gc.HasLen, 1)
}

func (s *MachinePortsDocSuite) TestOpenInvalidRange(c *gc.C) {
	toOpen := []network.PortRange{{FromPort: 400, ToPort: 200, Protocol: "tcp"}}
	err := s.openCloseMachinePorts(s.portsWithoutSubnet, s.unit1.Name(), toOpen, nil)
	c.Assert(err, gc.ErrorMatches, `cannot open ports 400-200/tcp: invalid port range 400-200/tcp`)
}

func (s *MachinePortsDocSuite) TestCloseInvalidRange(c *gc.C) {
	// Open initial port range
	toOpen := []network.PortRange{network.MustParsePortRange("100-200/tcp")}
	s.mustOpenCloseMachinePorts(c, s.portsWithoutSubnet, s.unit1.Name(), toOpen, nil)

	assertRefreshMachinePortsDoc(c, s.portsWithoutSubnet, nil)

	toClose := []network.PortRange{{FromPort: 150, ToPort: 200, Protocol: "tcp"}}
	err := s.openCloseMachinePorts(s.portsWithoutSubnet, s.unit1.Name(), nil, toClose)
	c.Assert(err, gc.ErrorMatches, `cannot close ports 150-200/tcp: port ranges 100-200/tcp \("wordpress/0"\) and 150-200/tcp \("wordpress/0"\) conflict`)
}

func (s *MachinePortsDocSuite) TestRemoveMachinePortsDoc(c *gc.C) {
	// Open initial port range
	toOpen := []network.PortRange{network.MustParsePortRange("100-200/tcp")}
	s.mustOpenCloseMachinePorts(c, s.portsWithoutSubnet, s.unit1.Name(), toOpen, nil)

	ports, err := s.machine.OpenedPortsInSubnet(s.subnet.ID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.NotNil)

	// Remove all port documents
	allMachinePorts, err := s.machine.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)

	for _, prt := range allMachinePorts {
		assertRemoveMachinePortsDoc(c, prt)
	}

	ports, err = s.machine.OpenedPortsInSubnet(s.subnet.ID())
	c.Assert(err, jc.ErrorIsNil)
	assertMachinePortsPersisted(c, ports, false) // we should get back a blank fresh doc
}

func (s *MachinePortsDocSuite) TestWatchMachinePorts(c *gc.C) {
	// No port ranges open initially, no changes.
	w := s.State.WatchOpenedPorts()
	c.Assert(w, gc.NotNil)

	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	// The first change we get is an empty one, as there are no ports
	// opened yet and we need an initial event for the API watcher to
	// work.
	wc.AssertChange()
	wc.AssertNoChange()

	portRange := network.MustParsePortRange("100-200/tcp")

	// Open a port range, detect a change.
	expectChange := fmt.Sprintf("%s:%s", s.machine.Id(), s.subnet.ID())
	s.mustOpenCloseMachinePorts(c, s.portsInSubnet, s.unit1.Name(), []network.PortRange{portRange}, nil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Close the port range, detect a change.
	s.mustOpenCloseMachinePorts(c, s.portsInSubnet, s.unit1.Name(), nil, []network.PortRange{portRange})
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Close the port range again, no changes.
	s.mustOpenCloseMachinePorts(c, s.portsInSubnet, s.unit1.Name(), nil, []network.PortRange{portRange})
	wc.AssertNoChange()

	// Open another range, detect a change.
	portRange = network.MustParsePortRange("999-1999/udp")
	s.mustOpenCloseMachinePorts(c, s.portsInSubnet, s.unit1.Name(), []network.PortRange{portRange}, nil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Open the same range again, no changes.
	s.mustOpenCloseMachinePorts(c, s.portsInSubnet, s.unit1.Name(), []network.PortRange{portRange}, nil)
	wc.AssertNoChange()

	// Open another range, detect a change.
	otherRange := network.MustParsePortRange("1-100/tcp")
	s.mustOpenCloseMachinePorts(c, s.portsInSubnet, s.unit1.Name(), []network.PortRange{otherRange}, nil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Close the other range, detect a change.
	s.mustOpenCloseMachinePorts(c, s.portsInSubnet, s.unit1.Name(), nil, []network.PortRange{otherRange})
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Remove the ports document, detect a change.
	assertRemoveMachinePortsDoc(c, s.portsInSubnet)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// And again - no change.
	assertRemoveMachinePortsDoc(c, s.portsInSubnet)
	wc.AssertNoChange()
}

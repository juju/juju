// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
)

type PortsDocSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit
	machine *state.Machine
	ports   *state.Ports
}

var _ = gc.Suite(&PortsDocSuite{})

func (s *PortsDocSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "wordpress")
	var err error
	s.service = s.AddTestingService(c, "wordpress", s.charm)
	c.Assert(err, gc.IsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	s.machine, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = s.unit.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)

	s.ports, err = state.GetOrCreatePorts(s.State, s.machine.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(s.ports, gc.NotNil)
}

func (s *PortsDocSuite) TestCreatePorts(c *gc.C) {
	ports, err := state.GetOrCreatePorts(s.State, s.machine.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.NotNil)
	err = ports.OpenPorts(state.PortRange{
		FromPort: 100,
		ToPort:   200,
		UnitName: s.unit.Name(),
		Protocol: "TCP",
	})
	c.Assert(err, gc.IsNil)

	ports, err = state.GetPorts(s.State, s.machine.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.NotNil)

	c.Assert(ports.PortsForUnit(s.unit.Name()), gc.HasLen, 1)
}

func (s *PortsDocSuite) TestOpenAndClosePorts(c *gc.C) {
	portRange := state.PortRange{
		FromPort: 100,
		ToPort:   200,
		UnitName: s.unit.Name(),
		Protocol: "TCP",
	}
	err := s.ports.OpenPorts(portRange)
	c.Assert(err, gc.IsNil)

	err = s.ports.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.ports.PortsForUnit(s.unit.Name()), gc.HasLen, 1)

	err = s.ports.ClosePorts(portRange)
	c.Assert(err, gc.IsNil)

	err = s.ports.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.ports.PortsForUnit(s.unit.Name()), gc.HasLen, 0)
}

func (s *PortsDocSuite) TestOpenInvalidRange(c *gc.C) {
	portRange := state.PortRange{
		FromPort: 400,
		ToPort:   200,
		UnitName: s.unit.Name(),
		Protocol: "TCP",
	}
	err := s.ports.OpenPorts(portRange)
	c.Assert(err, gc.ErrorMatches, "port range .* is invalid")
}

func (s *PortsDocSuite) TestCloseInvalidRange(c *gc.C) {
	portRange := state.PortRange{
		FromPort: 100,
		ToPort:   200,
		UnitName: s.unit.Name(),
		Protocol: "TCP",
	}
	err := s.ports.OpenPorts(portRange)
	c.Assert(err, gc.IsNil)

	err = s.ports.ClosePorts(state.PortRange{
		FromPort: 150,
		ToPort:   200,
		UnitName: s.unit.Name(),
		Protocol: "TCP",
	})
	c.Assert(err, gc.ErrorMatches, "no match found for port range: .*")
}

func (s *PortsDocSuite) TestRemovePortsDoc(c *gc.C) {
	portRange := state.PortRange{
		FromPort: 100,
		ToPort:   200,
		UnitName: s.unit.Name(),
		Protocol: "TCP",
	}
	err := s.ports.OpenPorts(portRange)
	c.Assert(err, gc.IsNil)

	ports, err := state.GetPorts(s.State, s.machine.Id())
	c.Assert(err, gc.IsNil)
	c.Assert(ports, gc.NotNil)

	allPorts, err := s.machine.OpenedPorts(s.State)
	c.Assert(err, gc.IsNil)

	for _, prt := range allPorts {
		err := prt.Remove()
		c.Assert(err, gc.IsNil)
	}

	ports, err = state.GetPorts(s.State, s.machine.Id())
	c.Assert(ports, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "ports document for machine .* not found")
}

type PortRangeSuite struct{}

var _ = gc.Suite(&PortRangeSuite{})

func (p *PortRangeSuite) TestPortRangeConflicts(c *gc.C) {
	var testCases = []struct {
		about          string
		first          state.PortRange
		second         state.PortRange
		expectConflict bool
	}{{
		"identical ports",
		state.PortRange{"wordpress/0", 80, 80, "TCP"},
		state.PortRange{"wordpress/0", 80, 80, "TCP"},
		true,
	}, {
		"different ports",
		state.PortRange{"wordpress/0", 80, 80, "TCP"},
		state.PortRange{"wordpress/0", 90, 90, "TCP"},
		false,
	}, {
		"touching ranges",
		state.PortRange{"wordpress/0", 100, 200, "TCP"},
		state.PortRange{"wordpress/0", 201, 240, "TCP"},
		false,
	}, {
		"touching ranges with overlap",
		state.PortRange{"wordpress/0", 100, 200, "TCP"},
		state.PortRange{"wordpress/0", 200, 240, "TCP"},
		true,
	}, {
		"different protocols",
		state.PortRange{"wordpress/0", 80, 80, "UDP"},
		state.PortRange{"wordpress/0", 80, 80, "TCP"},
		false,
	}, {
		"outside range",
		state.PortRange{"wordpress/0", 100, 200, "TCP"},
		state.PortRange{"wordpress/0", 80, 80, "TCP"},
		false,
	}, {
		"overlap end",
		state.PortRange{"wordpress/0", 100, 200, "TCP"},
		state.PortRange{"wordpress/0", 80, 120, "TCP"},
		true,
	}, {
		"complete overlap",
		state.PortRange{"wordpress/0", 100, 200, "TCP"},
		state.PortRange{"wordpress/0", 120, 140, "TCP"},
		true,
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		c.Check(t.first.ConflictsWith(t.second), gc.Equals, t.expectConflict)
		c.Check(t.second.ConflictsWith(t.first), gc.Equals, t.expectConflict)
	}
}

func (p *PortRangeSuite) TestPortRangeString(c *gc.C) {
	c.Assert(state.PortRange{"wordpress/0", 80, 80, "TCP"}.String(),
		gc.Equals,
		"80-80/tcp")
	c.Assert(state.PortRange{"wordpress/0", 80, 100, "TCP"}.String(),
		gc.Equals,
		"80-100/tcp")
}

func (p *PortRangeSuite) TestPortRangeValidity(c *gc.C) {
	testCases := []struct {
		about string
		ports state.PortRange
		valid bool
	}{{
		"single valid port",
		state.PortRange{"wordpress/0", 80, 80, "tcp"},
		true,
	}, {
		"valid port range",
		state.PortRange{"wordpress/0", 80, 90, "tcp"},
		true,
	}, {
		"valid udp port range",
		state.PortRange{"wordpress/0", 80, 90, "UDP"},
		true,
	}, {
		"invalid port range boundaries",
		state.PortRange{"wordpress/0", 90, 80, "tcp"},
		false,
	}, {
		"invalid protocol",
		state.PortRange{"wordpress/0", 80, 80, "some protocol"},
		false,
	}, {
		"invalid unit",
		state.PortRange{"invalid unit", 80, 80, "tcp"},
		false,
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		c.Assert(t.ports.IsValid(), gc.Equals, t.valid)
	}
}

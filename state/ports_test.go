// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type PortsDocSuite struct {
	ConnSuite
	charm              *state.Charm
	application        *state.Application
	unit1              *state.Unit
	unit2              *state.Unit
	machine            *state.Machine
	subnet             *state.Subnet
	portsOnSubnet      *state.Ports
	portsWithoutSubnet *state.Ports
}

var _ = gc.Suite(&PortsDocSuite{})

func (s *PortsDocSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)

	s.charm = s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	s.application = s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "wordpress", Charm: s.charm})
	s.machine = s.Factory.MakeMachine(c, &factory.MachineParams{Series: "quantal"})
	s.unit1 = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application, Machine: s.machine})
	s.unit2 = s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.application, Machine: s.machine})

	var err error
	s.subnet, err = s.State.AddSubnet(network.SubnetInfo{CIDR: "0.1.2.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	s.portsOnSubnet, err = state.GetOrCreatePorts(s.State, s.machine.Id(), s.subnet.CIDR())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.portsOnSubnet, gc.NotNil)

	s.portsWithoutSubnet, err = state.GetOrCreatePorts(s.State, s.machine.Id(), "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.portsWithoutSubnet, gc.NotNil)
}

func (s *PortsDocSuite) TestCreatePortsWithSubnet(c *gc.C) {
	s.testCreatePortsWithSubnetID(c, s.subnet.CIDR())
}

func (s *PortsDocSuite) testCreatePortsWithSubnetID(c *gc.C, subnetID string) {
	ports, err := state.GetOrCreatePorts(s.State, s.machine.Id(), subnetID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.NotNil)
	err = ports.OpenPorts(state.PortRange{
		FromPort: 100,
		ToPort:   200,
		UnitName: s.unit1.Name(),
		Protocol: "TCP",
	})
	c.Assert(err, jc.ErrorIsNil)

	ports, err = state.GetPorts(s.State, s.machine.Id(), subnetID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.NotNil)

	c.Assert(ports.PortsForUnit(s.unit1.Name()), gc.HasLen, 1)
}

func (s *PortsDocSuite) TestCreatePortsWithoutSubnet(c *gc.C) {
	s.testCreatePortsWithSubnetID(c, "")
}

func (s *PortsDocSuite) TestOpenAndClosePorts(c *gc.C) {

	testCases := []struct {
		about    string
		existing []state.PortRange
		open     *state.PortRange
		close    *state.PortRange
		expected string
	}{{
		about:    "open and close same port range",
		existing: nil,
		open: &state.PortRange{
			FromPort: 100,
			ToPort:   200,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		},
		close: &state.PortRange{
			FromPort: 100,
			ToPort:   200,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		},
		expected: "",
	}, {
		about:    "open and close icmp",
		existing: nil,
		open: &state.PortRange{
			FromPort: -1,
			ToPort:   -1,
			UnitName: s.unit1.Name(),
			Protocol: "ICMP",
		},
		close: &state.PortRange{
			FromPort: -1,
			ToPort:   -1,
			UnitName: s.unit1.Name(),
			Protocol: "ICMP",
		},
		expected: "",
	}, {
		about: "try to close part of a port range",
		existing: []state.PortRange{{
			FromPort: 100,
			ToPort:   200,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		}},
		open: nil,
		close: &state.PortRange{
			FromPort: 100,
			ToPort:   150,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		},
		expected: `cannot close ports 100-150/tcp \("wordpress/0"\): port ranges 100-200/tcp \("wordpress/0"\) and 100-150/tcp \("wordpress/0"\) conflict`,
	}, {
		about: "close an unopened port range with existing clash from other unit",
		existing: []state.PortRange{{
			FromPort: 100,
			ToPort:   150,
			UnitName: s.unit2.Name(),
			Protocol: "TCP",
		}},
		open: nil,
		close: &state.PortRange{
			FromPort: 100,
			ToPort:   150,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		},
		expected: "",
	}, {
		about: "open twice the same port range",
		existing: []state.PortRange{{
			FromPort: 100,
			ToPort:   150,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		}},
		open: &state.PortRange{
			FromPort: 100,
			ToPort:   150,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		},
		close:    nil,
		expected: "",
	}, {
		about:    "close an unopened port range",
		existing: nil,
		open:     nil,
		close: &state.PortRange{
			FromPort: 100,
			ToPort:   150,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		},
		expected: "",
	}, {
		about: "try to close an overlapping port range",
		existing: []state.PortRange{{
			FromPort: 100,
			ToPort:   200,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		}},
		open: nil,
		close: &state.PortRange{
			FromPort: 100,
			ToPort:   300,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		},
		expected: `cannot close ports 100-300/tcp \("wordpress/0"\): port ranges 100-200/tcp \("wordpress/0"\) and 100-300/tcp \("wordpress/0"\) conflict`,
	}, {
		about: "try to open an overlapping port range with different unit",
		existing: []state.PortRange{{
			FromPort: 100,
			ToPort:   200,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		}},
		open: &state.PortRange{
			FromPort: 100,
			ToPort:   300,
			UnitName: s.unit2.Name(),
			Protocol: "TCP",
		},
		expected: `cannot open ports 100-300/tcp \("wordpress/1"\): port ranges 100-200/tcp \("wordpress/0"\) and 100-300/tcp \("wordpress/1"\) conflict`,
	}, {
		about: "try to open an identical port range with different unit",
		existing: []state.PortRange{{
			FromPort: 100,
			ToPort:   200,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		}},
		open: &state.PortRange{
			FromPort: 100,
			ToPort:   200,
			UnitName: s.unit2.Name(),
			Protocol: "TCP",
		},
		expected: `cannot open ports 100-200/tcp \("wordpress/1"\): port ranges 100-200/tcp \("wordpress/0"\) and 100-200/tcp \("wordpress/1"\) conflict`,
	}, {
		about: "try to open a port range with different protocol with different unit",
		existing: []state.PortRange{{
			FromPort: 100,
			ToPort:   200,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		}},
		open: &state.PortRange{
			FromPort: 100,
			ToPort:   200,
			UnitName: s.unit2.Name(),
			Protocol: "UDP",
		},
		expected: "",
	}, {
		about: "try to open a non-overlapping port range with different unit",
		existing: []state.PortRange{{
			FromPort: 100,
			ToPort:   200,
			UnitName: s.unit1.Name(),
			Protocol: "TCP",
		}},
		open: &state.PortRange{
			FromPort: 300,
			ToPort:   400,
			UnitName: s.unit2.Name(),
			Protocol: "TCP",
		},
		expected: "",
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)

		ports, err := state.GetOrCreatePorts(s.State, s.machine.Id(), s.subnet.CIDR())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ports, gc.NotNil)

		// open ports that should exist for the test case
		for _, portRange := range t.existing {
			err := ports.OpenPorts(portRange)
			c.Check(err, jc.ErrorIsNil)
		}
		if t.existing != nil {
			err = ports.Refresh()
			c.Check(err, jc.ErrorIsNil)
		}
		if t.open != nil {
			err = ports.OpenPorts(*t.open)
			if t.expected == "" {
				c.Check(err, jc.ErrorIsNil)
			} else {
				c.Check(err, gc.ErrorMatches, t.expected)
			}
			err = ports.Refresh()
			c.Check(err, jc.ErrorIsNil)

		}

		if t.close != nil {
			err := ports.ClosePorts(*t.close)
			if t.expected == "" {
				c.Check(err, jc.ErrorIsNil)
			} else {
				c.Check(err, gc.ErrorMatches, t.expected)
			}
		}
		err = ports.Remove()
		c.Check(err, jc.ErrorIsNil)
	}
}

func (s *PortsDocSuite) TestAllPortRanges(c *gc.C) {
	portRange := state.PortRange{
		FromPort: 100,
		ToPort:   200,
		UnitName: s.unit1.Name(),
		Protocol: "TCP",
	}
	err := s.portsWithoutSubnet.OpenPorts(portRange)
	c.Assert(err, jc.ErrorIsNil)

	ranges := s.portsWithoutSubnet.AllPortRanges()
	c.Assert(ranges, gc.HasLen, 1)

	c.Assert(ranges[network.PortRange{100, 200, "TCP"}], gc.Equals, s.unit1.Name())
}

func (s *PortsDocSuite) TestICMP(c *gc.C) {
	portRange := state.PortRange{
		FromPort: -1,
		ToPort:   -1,
		UnitName: s.unit1.Name(),
		Protocol: "ICMP",
	}
	err := s.portsWithoutSubnet.OpenPorts(portRange)
	c.Assert(err, jc.ErrorIsNil)

	ranges := s.portsWithoutSubnet.AllPortRanges()
	c.Assert(ranges, gc.HasLen, 1)

	c.Assert(ranges[network.PortRange{-1, -1, "ICMP"}], gc.Equals, s.unit1.Name())
}

func (s *PortsDocSuite) TestOpenInvalidRange(c *gc.C) {
	portRange := state.PortRange{
		FromPort: 400,
		ToPort:   200,
		UnitName: s.unit1.Name(),
		Protocol: "TCP",
	}
	err := s.portsWithoutSubnet.OpenPorts(portRange)
	c.Assert(err, gc.ErrorMatches, `cannot open ports 400-200/tcp \("wordpress/0"\): invalid port range 400-200`)
}

func (s *PortsDocSuite) TestCloseInvalidRange(c *gc.C) {
	portRange := state.PortRange{
		FromPort: 100,
		ToPort:   200,
		UnitName: s.unit1.Name(),
		Protocol: "TCP",
	}
	err := s.portsWithoutSubnet.OpenPorts(portRange)
	c.Assert(err, jc.ErrorIsNil)

	err = s.portsWithoutSubnet.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = s.portsWithoutSubnet.ClosePorts(state.PortRange{
		FromPort: 150,
		ToPort:   200,
		UnitName: s.unit1.Name(),
		Protocol: "TCP",
	})
	c.Assert(err, gc.ErrorMatches, `cannot close ports 150-200/tcp \("wordpress/0"\): port ranges 100-200/tcp \("wordpress/0"\) and 150-200/tcp \("wordpress/0"\) conflict`)
}

func (s *PortsDocSuite) TestRemovePortsDoc(c *gc.C) {
	portRange := state.PortRange{
		FromPort: 100,
		ToPort:   200,
		UnitName: s.unit1.Name(),
		Protocol: "TCP",
	}
	err := s.portsOnSubnet.OpenPorts(portRange)
	c.Assert(err, jc.ErrorIsNil)

	ports, err := state.GetPorts(s.State, s.machine.Id(), s.subnet.CIDR())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.NotNil)

	allPorts, err := s.machine.AllPorts()
	c.Assert(err, jc.ErrorIsNil)

	for _, prt := range allPorts {
		err := prt.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}

	ports, err = state.GetPorts(s.State, s.machine.Id(), s.subnet.CIDR())
	c.Assert(ports, gc.IsNil)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(err, gc.ErrorMatches, `ports for machine "0", subnet "0.1.2.0/24" not found`)
}

func (s *PortsDocSuite) TestWatchPorts(c *gc.C) {
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

	portRange := state.PortRange{
		FromPort: 100,
		ToPort:   200,
		UnitName: s.unit1.Name(),
		Protocol: "TCP",
	}
	expectChange := fmt.Sprintf("%s:%s", s.machine.Id(), s.subnet.CIDR())
	// Open a port range, detect a change.
	err := s.portsOnSubnet.OpenPorts(portRange)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Close the port range, detect a change.
	err = s.portsOnSubnet.ClosePorts(portRange)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Close the port range again, no changes.
	err = s.portsOnSubnet.ClosePorts(portRange)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Open another range, detect a change.
	portRange = state.PortRange{
		FromPort: 999,
		ToPort:   1999,
		UnitName: s.unit2.Name(),
		Protocol: "udp",
	}
	err = s.portsOnSubnet.OpenPorts(portRange)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Open the same range again, no changes.
	err = s.portsOnSubnet.OpenPorts(portRange)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Open another range, detect a change.
	otherRange := state.PortRange{
		FromPort: 1,
		ToPort:   100,
		UnitName: s.unit1.Name(),
		Protocol: "tcp",
	}
	err = s.portsOnSubnet.OpenPorts(otherRange)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Close the other range, detect a change.
	err = s.portsOnSubnet.ClosePorts(otherRange)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// Remove the ports document, detect a change.
	err = s.portsOnSubnet.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(expectChange)
	wc.AssertNoChange()

	// And again - no change.
	err = s.portsOnSubnet.Remove()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

type PortRangeSuite struct{}

var _ = gc.Suite(&PortRangeSuite{})

// Create a port range or panic if it is invalid.
func MustPortRange(unitName string, fromPort, toPort int, protocol string) state.PortRange {
	portRange, err := state.NewPortRange(unitName, fromPort, toPort, protocol)
	if err != nil {
		panic(err)
	}
	return portRange
}

func (p *PortRangeSuite) TestPortRangeConflicts(c *gc.C) {
	var testCases = []struct {
		about    string
		first    state.PortRange
		second   state.PortRange
		expected interface{}
	}{{
		"identical ports",
		MustPortRange("wordpress/0", 80, 80, "TCP"),
		MustPortRange("wordpress/0", 80, 80, "TCP"),
		nil,
	}, {
		"identical port ranges",
		MustPortRange("wordpress/0", 80, 100, "TCP"),
		MustPortRange("wordpress/0", 80, 100, "TCP"),
		nil,
	}, {
		"different ports",
		MustPortRange("wordpress/0", 80, 80, "TCP"),
		MustPortRange("wordpress/0", 90, 90, "TCP"),
		nil,
	}, {
		"touching ranges",
		MustPortRange("wordpress/0", 100, 200, "TCP"),
		MustPortRange("wordpress/0", 201, 240, "TCP"),
		nil,
	}, {
		"touching ranges with overlap",
		MustPortRange("wordpress/0", 100, 200, "TCP"),
		MustPortRange("wordpress/0", 200, 240, "TCP"),
		"port ranges .* conflict",
	}, {
		"identical ports with different protocols",
		MustPortRange("wordpress/0", 80, 80, "UDP"),
		MustPortRange("wordpress/0", 80, 80, "TCP"),
		nil,
	}, {
		"overlapping ranges with different protocols",
		MustPortRange("wordpress/0", 80, 200, "UDP"),
		MustPortRange("wordpress/0", 80, 300, "TCP"),
		nil,
	}, {
		"outside range",
		MustPortRange("wordpress/0", 100, 200, "TCP"),
		MustPortRange("wordpress/0", 80, 80, "TCP"),
		nil,
	}, {
		"overlap end",
		MustPortRange("wordpress/0", 100, 200, "TCP"),
		MustPortRange("wordpress/0", 80, 120, "TCP"),
		"port ranges .* conflict",
	}, {
		"complete overlap",
		MustPortRange("wordpress/0", 100, 200, "TCP"),
		MustPortRange("wordpress/0", 120, 140, "TCP"),
		"port ranges .* conflict",
	}, {
		"overlap with same end",
		MustPortRange("wordpress/0", 100, 200, "TCP"),
		MustPortRange("wordpress/0", 120, 200, "TCP"),
		"port ranges .* conflict",
	}, {
		"overlap with same start",
		MustPortRange("wordpress/0", 100, 200, "TCP"),
		MustPortRange("wordpress/0", 100, 120, "TCP"),
		"port ranges .* conflict",
	}, {
		"invalid port range",
		state.PortRange{"wordpress/0", 100, 80, "TCP"},
		MustPortRange("wordpress/0", 80, 80, "TCP"),
		"invalid port range 100-80",
	}, {
		"different units, same port",
		MustPortRange("mysql/0", 80, 80, "TCP"),
		MustPortRange("wordpress/0", 80, 80, "TCP"),
		"port ranges .* conflict",
	}, {
		"different units, different port ranges",
		MustPortRange("mysql/0", 80, 100, "TCP"),
		MustPortRange("wordpress/0", 180, 280, "TCP"),
		nil,
	}, {
		"different units, overlapping port ranges",
		MustPortRange("mysql/0", 80, 100, "TCP"),
		MustPortRange("wordpress/0", 90, 280, "TCP"),
		"port ranges .* conflict",
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		if t.expected == nil {
			c.Check(t.first.CheckConflicts(t.second), gc.IsNil)
			c.Check(t.second.CheckConflicts(t.first), gc.IsNil)
		} else if _, isString := t.expected.(string); isString {
			c.Check(t.first.CheckConflicts(t.second), gc.ErrorMatches, t.expected.(string))
			c.Check(t.second.CheckConflicts(t.first), gc.ErrorMatches, t.expected.(string))
		}
		// change test case protocols and test again
		c.Logf("test %d: %s (after protocol swap)", i, t.about)
		t.first.Protocol = swapProtocol(t.first.Protocol)
		t.second.Protocol = swapProtocol(t.second.Protocol)
		c.Logf("%+v %+v %v", t.first, t.second, t.expected)
		if t.expected == nil {
			c.Check(t.first.CheckConflicts(t.second), gc.IsNil)
			c.Check(t.second.CheckConflicts(t.first), gc.IsNil)
		} else if _, isString := t.expected.(string); isString {
			c.Check(t.first.CheckConflicts(t.second), gc.ErrorMatches, t.expected.(string))
			c.Check(t.second.CheckConflicts(t.first), gc.ErrorMatches, t.expected.(string))
		}

	}
}

func swapProtocol(protocol string) string {
	if strings.ToLower(protocol) == "tcp" {
		return "udp"
	}
	if strings.ToLower(protocol) == "udp" {
		return "tcp"
	}
	return protocol
}

func (p *PortRangeSuite) TestPortRangeString(c *gc.C) {
	c.Assert(state.PortRange{"wordpress/42", 80, 80, "TCP"}.String(),
		gc.Equals,
		`80-80/tcp ("wordpress/42")`,
	)
	c.Assert(state.PortRange{"wordpress/0", 80, 100, "TCP"}.String(),
		gc.Equals,
		`80-100/tcp ("wordpress/0")`,
	)
	c.Assert(state.PortRange{"wordpress/0", -1, -1, "ICMP"}.String(),
		gc.Equals,
		`icmp ("wordpress/0")`,
	)
}

func (p *PortRangeSuite) TestPortRangeValidityAndLength(c *gc.C) {
	testCases := []struct {
		about        string
		ports        state.PortRange
		expectLength int
		expectedErr  string
	}{{
		"single valid port",
		state.PortRange{"wordpress/0", 80, 80, "tcp"},
		1,
		"",
	}, {
		"valid tcp port range",
		state.PortRange{"wordpress/0", 80, 90, "tcp"},
		11,
		"",
	}, {
		"valid udp port range",
		state.PortRange{"wordpress/0", 80, 90, "UDP"},
		11,
		"",
	}, {
		"invalid port range boundaries",
		state.PortRange{"wordpress/0", 90, 80, "tcp"},
		0,
		"invalid port range.*",
	}, {
		"invalid protocol",
		state.PortRange{"wordpress/0", 80, 80, "some protocol"},
		0,
		"invalid protocol.*",
	}, {
		"invalid unit",
		state.PortRange{"invalid unit", 80, 80, "tcp"},
		0,
		"invalid unit.*",
	}, {
		"negative lower bound",
		state.PortRange{"wordpress/0", -10, 10, "tcp"},
		0,
		"port range bounds must be between 1 and 65535.*",
	}, {
		"zero lower bound",
		state.PortRange{"wordpress/0", 0, 10, "tcp"},
		0,
		"port range bounds must be between 1 and 65535.*",
	}, {
		"negative upper bound",
		state.PortRange{"wordpress/0", 10, -10, "tcp"},
		0,
		"invalid port range.*",
	}, {
		"zero upper bound",
		state.PortRange{"wordpress/0", 10, 0, "tcp"},
		0,
		"invalid port range.*",
	}, {
		"too large lower bound",
		state.PortRange{"wordpress/0", 65540, 99999, "tcp"},
		0,
		"port range bounds must be between 1 and 65535.*",
	}, {
		"too large upper bound",
		state.PortRange{"wordpress/0", 10, 99999, "tcp"},
		0,
		"port range bounds must be between 1 and 65535.*",
	}, {
		"longest valid range",
		state.PortRange{"wordpress/0", 1, 65535, "tcp"},
		65535,
		"",
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		c.Check(t.ports.Length(), gc.Equals, t.expectLength)
		if t.expectedErr == "" {
			c.Check(t.ports.Validate(), gc.IsNil)
		} else {
			c.Check(t.ports.Validate(), gc.ErrorMatches, t.expectedErr)
		}
	}
}

func (p *PortRangeSuite) TestSanitizeBounds(c *gc.C) {
	tests := []struct {
		about  string
		input  state.PortRange
		output state.PortRange
	}{{
		"valid range",
		state.PortRange{"", 100, 200, ""},
		state.PortRange{"", 100, 200, ""},
	}, {
		"negative lower bound",
		state.PortRange{"", -10, 10, ""},
		state.PortRange{"", 1, 10, ""},
	}, {
		"zero lower bound",
		state.PortRange{"", 0, 10, ""},
		state.PortRange{"", 1, 10, ""},
	}, {
		"negative upper bound",
		state.PortRange{"", 42, -20, ""},
		state.PortRange{"", 1, 42, ""},
	}, {
		"zero upper bound",
		state.PortRange{"", 42, 0, ""},
		state.PortRange{"", 1, 42, ""},
	}, {
		"both bounds negative",
		state.PortRange{"", -10, -20, ""},
		state.PortRange{"", 1, 1, ""},
	}, {
		"both bounds zero",
		state.PortRange{"", 0, 0, ""},
		state.PortRange{"", 1, 1, ""},
	}, {
		"swapped bounds",
		state.PortRange{"", 20, 10, ""},
		state.PortRange{"", 10, 20, ""},
	}, {
		"too large upper bound",
		state.PortRange{"", 20, 99999, ""},
		state.PortRange{"", 20, 65535, ""},
	}, {
		"too large lower bound",
		state.PortRange{"", 99999, 10, ""},
		state.PortRange{"", 10, 65535, ""},
	}, {
		"both bounds too large",
		state.PortRange{"", 88888, 99999, ""},
		state.PortRange{"", 65535, 65535, ""},
	}, {
		"lower negative, upper too large",
		state.PortRange{"", -10, 99999, ""},
		state.PortRange{"", 1, 65535, ""},
	}, {
		"lower zero, upper too large",
		state.PortRange{"", 0, 99999, ""},
		state.PortRange{"", 1, 65535, ""},
	}}
	for i, t := range tests {
		c.Logf("test %d: %s", i, t.about)
		c.Check(t.input.SanitizeBounds(), jc.DeepEquals, t.output)
	}
}

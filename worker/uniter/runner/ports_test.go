// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"github.com/juju/names"
	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/uniter/runner"
)

type PortsSuite struct {
	envtesting.IsolationSuite
}

var _ = gc.Suite(&PortsSuite{})

func (s *PortsSuite) TestValidatePortRange(c *gc.C) {
	tests := []struct {
		about     string
		proto     string
		ports     []int
		portRange network.PortRange
		expectErr string
	}{{
		about:     "invalid range - 0-0/tcp",
		proto:     "tcp",
		ports:     []int{0, 0},
		expectErr: "invalid port range 0-0/tcp",
	}, {
		about:     "invalid range - 0-1/tcp",
		proto:     "tcp",
		ports:     []int{0, 1},
		expectErr: "invalid port range 0-1/tcp",
	}, {
		about:     "invalid range - -1-1/tcp",
		proto:     "tcp",
		ports:     []int{-1, 1},
		expectErr: "invalid port range -1-1/tcp",
	}, {
		about:     "invalid range - 1-99999/tcp",
		proto:     "tcp",
		ports:     []int{1, 99999},
		expectErr: "invalid port range 1-99999/tcp",
	}, {
		about:     "invalid range - 88888-99999/tcp",
		proto:     "tcp",
		ports:     []int{88888, 99999},
		expectErr: "invalid port range 88888-99999/tcp",
	}, {
		about:     "invalid protocol - 1-65535/foo",
		proto:     "foo",
		ports:     []int{1, 65535},
		expectErr: `invalid protocol "foo", expected "tcp" or "udp"`,
	}, {
		about: "valid range - 100-200/udp",
		proto: "UDP",
		ports: []int{100, 200},
		portRange: network.PortRange{
			FromPort: 100,
			ToPort:   200,
			Protocol: "udp",
		},
	}, {
		about: "valid single port - 100/tcp",
		proto: "TCP",
		ports: []int{100, 100},
		portRange: network.PortRange{
			FromPort: 100,
			ToPort:   100,
			Protocol: "tcp",
		},
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)
		portRange, err := runner.ValidatePortRange(
			test.proto,
			test.ports[0],
			test.ports[1],
		)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
			c.Check(portRange, jc.DeepEquals, network.PortRange{})
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(portRange, jc.DeepEquals, test.portRange)
		}
	}
}

func makeMachinePorts(
	unitName, proto string, fromPort, toPort int,
) map[network.PortRange]params.RelationUnit {
	result := make(map[network.PortRange]params.RelationUnit)
	portRange := network.PortRange{
		FromPort: fromPort,
		ToPort:   toPort,
		Protocol: proto,
	}
	unitTag := ""
	if unitName != "invalid" {
		unitTag = names.NewUnitTag(unitName).String()
	} else {
		unitTag = unitName
	}
	result[portRange] = params.RelationUnit{
		Unit: unitTag,
	}
	return result
}

func makePendingPorts(
	proto string, fromPort, toPort int, shouldOpen bool,
) map[runner.PortRange]runner.PortRangeInfo {
	result := make(map[runner.PortRange]runner.PortRangeInfo)
	portRange := network.PortRange{
		FromPort: fromPort,
		ToPort:   toPort,
		Protocol: proto,
	}
	key := runner.PortRange{
		Ports:      portRange,
		RelationId: -1,
	}
	result[key] = runner.PortRangeInfo{
		ShouldOpen: shouldOpen,
	}
	return result
}

type portsTest struct {
	about         string
	proto         string
	ports         []int
	machinePorts  map[network.PortRange]params.RelationUnit
	pendingPorts  map[runner.PortRange]runner.PortRangeInfo
	expectErr     string
	expectPending map[runner.PortRange]runner.PortRangeInfo
}

func (p portsTest) withDefaults(proto string, fromPort, toPort int) portsTest {
	if p.proto == "" {
		p.proto = proto
	}
	if len(p.ports) != 2 {
		p.ports = []int{fromPort, toPort}
	}
	if p.pendingPorts == nil {
		p.pendingPorts = make(map[runner.PortRange]runner.PortRangeInfo)
	}
	return p
}

func (s *PortsSuite) TestTryOpenPorts(c *gc.C) {
	tests := []portsTest{{
		about:     "invalid port range",
		ports:     []int{0, 0},
		expectErr: "invalid port range 0-0/tcp",
	}, {
		about:     "invalid protocol - 10-20/foo",
		proto:     "foo",
		expectErr: `invalid protocol "foo", expected "tcp" or "udp"`,
	}, {
		about:         "open a new range (no machine ports yet)",
		expectPending: makePendingPorts("tcp", 10, 20, true),
	}, {
		about:         "open an existing range (ignored)",
		machinePorts:  makeMachinePorts("u/0", "tcp", 10, 20),
		expectPending: map[runner.PortRange]runner.PortRangeInfo{},
	}, {
		about:         "open a range pending to be closed already",
		pendingPorts:  makePendingPorts("tcp", 10, 20, false),
		expectPending: makePendingPorts("tcp", 10, 20, true),
	}, {
		about:         "open a range pending to be opened already (ignored)",
		pendingPorts:  makePendingPorts("tcp", 10, 20, true),
		expectPending: makePendingPorts("tcp", 10, 20, true),
	}, {
		about:        "try opening a range when machine ports has invalid unit tag",
		machinePorts: makeMachinePorts("invalid", "tcp", 80, 90),
		expectErr:    `machine ports 80-90/tcp contain invalid unit tag: "invalid" is not a valid tag`,
	}, {
		about:        "try opening a range conflicting with another unit",
		machinePorts: makeMachinePorts("u/1", "tcp", 10, 20),
		expectErr:    `cannot open 10-20/tcp \(unit "u/0"\): conflicts with existing 10-20/tcp \(unit "u/1"\)`,
	}, {
		about:         "open a range conflicting with the same unit (ignored)",
		machinePorts:  makeMachinePorts("u/0", "tcp", 10, 20),
		expectPending: map[runner.PortRange]runner.PortRangeInfo{},
	}, {
		about:        "try opening a range conflicting with another pending range",
		pendingPorts: makePendingPorts("tcp", 5, 25, true),
		expectErr:    `cannot open 10-20/tcp \(unit "u/0"\): conflicts with 5-25/tcp requested earlier`,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)

		test = test.withDefaults("tcp", 10, 20)
		err := runner.TryOpenPorts(
			test.proto,
			test.ports[0],
			test.ports[1],
			names.NewUnitTag("u/0"),
			test.machinePorts,
			test.pendingPorts,
		)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(test.pendingPorts, jc.DeepEquals, test.expectPending)
		}
	}
}

func (s *PortsSuite) TestTryClosePorts(c *gc.C) {
	tests := []portsTest{{
		about:     "invalid port range",
		ports:     []int{0, 0},
		expectErr: "invalid port range 0-0/tcp",
	}, {
		about:     "invalid protocol - 10-20/foo",
		proto:     "foo",
		expectErr: `invalid protocol "foo", expected "tcp" or "udp"`,
	}, {
		about:         "close a new range (no machine ports yet; ignored)",
		expectPending: map[runner.PortRange]runner.PortRangeInfo{},
	}, {
		about:         "close an existing range",
		machinePorts:  makeMachinePorts("u/0", "tcp", 10, 20),
		expectPending: makePendingPorts("tcp", 10, 20, false),
	}, {
		about:         "close a range pending to be opened already (removed from pending)",
		pendingPorts:  makePendingPorts("tcp", 10, 20, true),
		expectPending: map[runner.PortRange]runner.PortRangeInfo{},
	}, {
		about:         "close a range pending to be closed already (ignored)",
		pendingPorts:  makePendingPorts("tcp", 10, 20, false),
		expectPending: makePendingPorts("tcp", 10, 20, false),
	}, {
		about:        "try closing an existing range when machine ports has invalid unit tag",
		machinePorts: makeMachinePorts("invalid", "tcp", 10, 20),
		expectErr:    `machine ports 10-20/tcp contain invalid unit tag: "invalid" is not a valid tag`,
	}, {
		about:        "try closing a range of another unit",
		machinePorts: makeMachinePorts("u/1", "tcp", 10, 20),
		expectErr:    `cannot close 10-20/tcp \(opened by "u/1"\) from "u/0"`,
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)

		test = test.withDefaults("tcp", 10, 20)
		err := runner.TryClosePorts(
			test.proto,
			test.ports[0],
			test.ports[1],
			names.NewUnitTag("u/0"),
			test.machinePorts,
			test.pendingPorts,
		)
		if test.expectErr != "" {
			c.Check(err, gc.ErrorMatches, test.expectErr)
		} else {
			c.Check(err, jc.ErrorIsNil)
			c.Check(test.pendingPorts, jc.DeepEquals, test.expectPending)
		}
	}
}

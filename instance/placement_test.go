// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
)

type PlacementSuite struct{}

var _ = gc.Suite(&PlacementSuite{})

var parsePlacementTests = []struct {
	arg    string
	expect *instance.Placement
	err    string
}{{
	arg:    "",
	expect: nil,
}, {
	arg:    "0",
	expect: &instance.Placement{Scope: instance.MachineScope, Value: "0"},
}, {
	arg:    "0/lxc/0",
	expect: &instance.Placement{Scope: instance.MachineScope, Value: "0/lxc/0"},
}, {
	arg: "#:x",
	err: `invalid value "x" for "#" scope: expected machine-id`,
}, {
	arg: "lxc:x",
	err: `invalid value "x" for "lxc" scope: expected machine-id`,
}, {
	arg: "kvm:x",
	err: `invalid value "x" for "kvm" scope: expected machine-id`,
}, {
	arg:    "kvm:123",
	expect: &instance.Placement{Scope: string(instance.KVM), Value: "123"},
}, {
	arg:    "lxc",
	expect: &instance.Placement{Scope: string(instance.LXC)},
}, {
	arg:    "non-standard",
	expect: &instance.Placement{Value: "non-standard"},
}, {
	arg:    ":non-standard",
	expect: &instance.Placement{Value: "non-standard"},
}, {
	arg:    "non:standard",
	expect: &instance.Placement{Scope: "non", Value: "standard"},
}}

func (s *PlacementSuite) TestParsePlacement(c *gc.C) {
	for i, t := range parsePlacementTests {
		c.Logf("test %d: %s", i, t.arg)
		p, err := instance.ParsePlacement(t.arg)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(p, gc.DeepEquals, t.expect)
	}
}

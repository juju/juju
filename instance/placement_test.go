// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
)

type PlacementSuite struct{}

var _ = gc.Suite(&PlacementSuite{})

func (s *PlacementSuite) TestParsePlacement(c *gc.C) {
	parsePlacementTests := []struct {
		arg                          string
		expectScope, expectDirective string
		err                          string
	}{{
		arg: "",
	}, {
		arg:             "0",
		expectScope:     instance.MachineScope,
		expectDirective: "0",
	}, {
		arg:             "0/lxc/0",
		expectScope:     instance.MachineScope,
		expectDirective: "0/lxc/0",
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
		arg:             "kvm:123",
		expectScope:     string(instance.KVM),
		expectDirective: "123",
	}, {
		arg:         "lxc",
		expectScope: string(instance.LXC),
	}, {
		arg: "non-standard",
		err: "placement scope missing",
	}, {
		arg: ":non-standard",
		err: "placement scope missing",
	}, {
		arg:             "non:standard",
		expectScope:     "non",
		expectDirective: "standard",
	}}

	for i, t := range parsePlacementTests {
		c.Logf("test %d: %s", i, t.arg)
		p, err := instance.ParsePlacement(t.arg)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, gc.IsNil)
		}
		if t.expectScope == "" && t.expectDirective == "" {
			c.Assert(p, gc.IsNil)
		} else {
			c.Assert(p, gc.DeepEquals, &instance.Placement{
				Scope:     t.expectScope,
				Directive: t.expectDirective,
			})
		}
	}
}

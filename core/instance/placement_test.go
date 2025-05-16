// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
)

type PlacementSuite struct{}

func TestPlacementSuite(t *stdtesting.T) { tc.Run(t, &PlacementSuite{}) }
func (s *PlacementSuite) TestParsePlacement(c *tc.C) {
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
		arg:             "0/lxd/0",
		expectScope:     instance.MachineScope,
		expectDirective: "0/lxd/0",
	}, {
		arg:             "lxd:0",
		expectScope:     string(instance.LXD),
		expectDirective: "0",
	}, {
		arg: "#:x",
		err: `invalid value "x" for "#" scope: expected machine-id`,
	}, {
		arg: "lxd:x",
		err: `invalid value "x" for "lxd" scope: expected machine-id`,
	}, {
		arg:         "lxd",
		expectScope: string(instance.LXD),
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
			c.Assert(err, tc.ErrorMatches, t.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
		}
		if t.expectScope == "" && t.expectDirective == "" {
			c.Assert(p, tc.IsNil)
		} else {
			c.Assert(p, tc.DeepEquals, &instance.Placement{
				Scope:     t.expectScope,
				Directive: t.expectDirective,
			})
		}
	}
}

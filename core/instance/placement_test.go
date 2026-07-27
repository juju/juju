// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
)

type PlacementSuite struct{}

func TestPlacementSuite(t *testing.T) {
	tc.Run(t, &PlacementSuite{})
}

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
	}, {
		arg:             "model-uuid:zone=us-east-1a",
		expectScope:     instance.ModelScope,
		expectDirective: "zone=us-east-1a",
	}, {
		arg:             "model-uuid:subnet=subnet-123",
		expectScope:     instance.ModelScope,
		expectDirective: "subnet=subnet-123",
	}, {
		arg:             "model-uuid:system-id=node-1",
		expectScope:     instance.ModelScope,
		expectDirective: "system-id=node-1",
	}, {
		arg:             "model-uuid:foo:bar",
		expectScope:     instance.ModelScope,
		expectDirective: "foo:bar",
	}, {
		arg:         "model-uuid:",
		expectScope: instance.ModelScope,
	}, {
		// A raw UUID scope (e.g. after the client substitutes the
		// "model-uuid" placeholder with the real model UUID) is
		// syntactically valid. The parser treats it as a generic
		// scope:directive pair without interpreting its semantics.
		arg:             "32c5aaae-6713-4cd7-83a4-d1256e9c97d0:zone=us-east-1a",
		expectScope:     "32c5aaae-6713-4cd7-83a4-d1256e9c97d0",
		expectDirective: "zone=us-east-1a",
	}, {
		// A bare "model-uuid" without a colon is not a valid
		// scope:directive pair, so it should fail.
		arg: "model-uuid",
		err: "placement scope missing",
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

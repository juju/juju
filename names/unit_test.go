// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
)

type unitSuite struct{}

var _ = gc.Suite(&unitSuite{})

func (s *unitSuite) TestUnitNameFromTag(c *gc.C) {
	// Try both valid and invalid tag formats.
	tag, err := names.UnitNameFromTag("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	c.Assert(tag, gc.Equals, "wordpress/0")

	tag, err = names.UnitNameFromTag("foo")
	c.Assert(err, gc.ErrorMatches, "invalid unit tag format: foo")
	c.Assert(tag, gc.Equals, "")
}

func (s *unitSuite) TestUnitTag(c *gc.C) {
	c.Assert(names.UnitTag("wordpress/2"), gc.Equals, "unit-wordpress-2")
}

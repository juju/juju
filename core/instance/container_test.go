// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

type InstanceSuite struct{}

var _ = tc.Suite(&InstanceSuite{})

func (s *InstanceSuite) TestParseContainerType(c *tc.C) {
	ctype, err := instance.ParseContainerType("lxd")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctype, tc.Equals, instance.LXD)

	ctype, err = instance.ParseContainerType("lxd")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctype, tc.Equals, instance.LXD)

	_, err = instance.ParseContainerType("none")
	c.Assert(err, tc.ErrorMatches, `invalid container type "none"`)

	_, err = instance.ParseContainerType("omg")
	c.Assert(err, tc.ErrorMatches, `invalid container type "omg"`)
}

func (s *InstanceSuite) TestParseContainerTypeOrNone(c *tc.C) {
	ctype, err := instance.ParseContainerTypeOrNone("lxd")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctype, tc.Equals, instance.LXD)

	ctype, err = instance.ParseContainerTypeOrNone("lxd")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctype, tc.Equals, instance.LXD)

	ctype, err = instance.ParseContainerTypeOrNone("none")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ctype, tc.Equals, instance.NONE)

	_, err = instance.ParseContainerTypeOrNone("omg")
	c.Assert(err, tc.ErrorMatches, `invalid container type "omg"`)
}

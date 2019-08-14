// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/testing"
)

type spaceSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&spaceSuite{})

func (*spaceSuite) TestString(c *gc.C) {
	result := network.SpaceInfos{
		{Name: "space1"},
		{Name: "space2"},
		{Name: "space3"},
	}.String()

	c.Assert(result, gc.Equals, "space1, space2, space3")
}

func (*spaceSuite) TestHasSpaceWithName(c *gc.C) {
	spaces := network.SpaceInfos{
		{Name: "space1"},
		{Name: "space2"},
		{Name: "space3"},
	}

	c.Assert(spaces.HasSpaceWithName("space1"), jc.IsTrue)
	c.Assert(spaces.HasSpaceWithName("space666"), jc.IsFalse)
}

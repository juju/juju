// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"github.com/juju/juju/api/common/charm"
	corecharm "github.com/juju/juju/core/charm"
	gc "gopkg.in/check.v1"
)

type originSuite struct{}

var _ = gc.Suite(&originSuite{})

func (originSuite) TestCoreChannel(c *gc.C) {
	track := "latest"
	origin := charm.Origin{
		Risk:  "edge",
		Track: &track,
	}
	c.Assert(origin.CoreChannel(), gc.DeepEquals, corecharm.Channel{
		Risk:  corecharm.Edge,
		Track: "latest",
	})
}

func (originSuite) TestCoreChannelWithEmptyTrack(c *gc.C) {
	origin := charm.Origin{
		Risk: "edge",
	}
	c.Assert(origin.CoreChannel(), gc.DeepEquals, corecharm.Channel{
		Risk: corecharm.Edge,
	})
}

func (originSuite) TestCoreChannelThatIsEmpty(c *gc.C) {
	origin := charm.Origin{}
	c.Assert(origin.CoreChannel(), gc.DeepEquals, corecharm.Channel{})
}

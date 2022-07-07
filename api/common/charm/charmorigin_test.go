// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"github.com/juju/charm/v9"
	gc "gopkg.in/check.v1"

	commoncharm "github.com/juju/juju/api/common/charm"
)

type originSuite struct{}

var _ = gc.Suite(&originSuite{})

func (originSuite) TestCoreChannel(c *gc.C) {
	track := "latest"
	branch := "foo"
	origin := commoncharm.Origin{
		Risk:   "edge",
		Track:  &track,
		Branch: &branch,
	}
	c.Assert(origin.CharmChannel(), gc.DeepEquals, charm.Channel{
		Risk:   charm.Edge,
		Track:  "latest",
		Branch: "foo",
	})
}

func (originSuite) TestCoreChannelWithEmptyTrack(c *gc.C) {
	origin := commoncharm.Origin{
		Risk: "edge",
	}
	c.Assert(origin.CharmChannel(), gc.DeepEquals, charm.Channel{
		Risk: charm.Edge,
	})
}

func (originSuite) TestCoreChannelThatIsEmpty(c *gc.C) {
	origin := commoncharm.Origin{}
	c.Assert(origin.CharmChannel(), gc.DeepEquals, charm.Channel{})
}

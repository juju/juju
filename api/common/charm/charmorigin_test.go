// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"github.com/juju/charm/v10"
	gc "gopkg.in/check.v1"

	commoncharm "github.com/juju/juju/api/common/charm"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
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

func (originSuite) TestConvertToCoreCharmOrigin(c *gc.C) {
	track := "latest"
	origin := commoncharm.Origin{
		Source:       "charm-hub",
		ID:           "foobar",
		Track:        &track,
		Risk:         "stable",
		Branch:       nil,
		Architecture: "amd64",
		Base:         corebase.MakeDefaultBase("ubuntu", "20.04"),
	}

	c.Assert(origin.CoreCharmOrigin(), gc.DeepEquals, corecharm.Origin{
		Source: "charm-hub",
		ID:     "foobar",
		Channel: &charm.Channel{
			Track: "latest",
			Risk:  "stable",
		},
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "20.04",
		},
	})
}

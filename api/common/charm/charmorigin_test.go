// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	commoncharm "github.com/juju/juju/api/common/charm"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm"
)

type originSuite struct{}

func TestOriginSuite(t *stdtesting.T) {
	tc.Run(t, &originSuite{})
}

func (originSuite) TestCoreChannel(c *tc.C) {
	track := "latest"
	branch := "foo"
	origin := commoncharm.Origin{
		Risk:   "edge",
		Track:  &track,
		Branch: &branch,
	}
	c.Assert(origin.CharmChannel(), tc.DeepEquals, charm.Channel{
		Risk:   charm.Edge,
		Track:  "latest",
		Branch: "foo",
	})
}

func (originSuite) TestCoreChannelWithEmptyTrack(c *tc.C) {
	origin := commoncharm.Origin{
		Risk: "edge",
	}
	c.Assert(origin.CharmChannel(), tc.DeepEquals, charm.Channel{
		Risk: charm.Edge,
	})
}

func (originSuite) TestCoreChannelThatIsEmpty(c *tc.C) {
	origin := commoncharm.Origin{}
	c.Assert(origin.CharmChannel(), tc.DeepEquals, charm.Channel{})
}

func (originSuite) TestConvertToCoreCharmOrigin(c *tc.C) {
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

	c.Assert(origin.CoreCharmOrigin(), tc.DeepEquals, corecharm.Origin{
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

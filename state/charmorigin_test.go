// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
)

type CharmOriginSuite struct {
}

var _ = gc.Suite(&CharmOriginSuite{})

func (s *CharmOriginSuite) TestValidateLocal(c *gc.C) {
	origin := successfulLocalOrigin()
	err := origin.validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CharmOriginSuite) TestValidateLocalFailChannel(c *gc.C) {
	origin := successfulLocalOrigin()
	origin.Channel = &Channel{Risk: "stable"}
	err := origin.validate()
	c.Assert(err, gc.NotNil)
}

func (s *CharmOriginSuite) TestValidateCharmhub(c *gc.C) {
	origin := successfulCharmHubOrigin()
	err := origin.validate()
	c.Assert(err, jc.ErrorIsNil)
}

func intPtr(i int) *int {
	return &i
}

func successfulCharmHubOrigin() CharmOrigin {
	return CharmOrigin{
		Source:   corecharm.CharmHub.String(),
		Type:     "charm",
		Revision: intPtr(7),
		Channel: &Channel{
			Risk: "stable",
		},
		Platform: &Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	}
}

func successfulLocalOrigin() CharmOrigin {
	return CharmOrigin{
		Source: corecharm.Local.String(),
		Platform: &Platform{
			Architecture: arch.DefaultArchitecture,
			OS:           "ubuntu",
			Channel:      "22.04",
		},
	}
}

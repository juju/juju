// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/charm/v11"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/rpc/params"
)

type clientNormalizeOriginSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&clientNormalizeOriginSuite{})

func (s *clientNormalizeOriginSuite) TestNormalizeCharmOriginNoAll(c *gc.C) {
	track := "1.0"
	branch := "foo"
	origin := params.CharmOrigin{
		Source:       "charm-hub",
		Type:         "charm",
		Risk:         "edge",
		Track:        &track,
		Branch:       &branch,
		Architecture: "all",
	}
	obtained, err := normalizeCharmOrigin(origin, "amd64")
	c.Assert(err, jc.ErrorIsNil)
	origin.Architecture = "amd64"
	c.Assert(obtained, gc.DeepEquals, origin)
}

func (s *clientNormalizeOriginSuite) TestNormalizeCharmOriginWithEmpty(c *gc.C) {
	track := "1.0"
	origin := params.CharmOrigin{
		Source:       "charm-hub",
		Type:         "charm",
		Risk:         "edge",
		Track:        &track,
		Architecture: "",
		Base:         params.Base{Channel: "all"},
	}
	obtained, err := normalizeCharmOrigin(origin, "amd64")
	c.Assert(err, jc.ErrorIsNil)
	origin.Architecture = "amd64"
	origin.Base.Channel = ""
	c.Assert(obtained, gc.DeepEquals, origin)
}

type clientValidateOriginSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&clientValidateOriginSuite{})

func (s *clientValidateOriginSuite) TestValidateOrigin(c *gc.C) {
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Platform: corecharm.Platform{Architecture: "all"},
	}

	err := validateOrigin(origin, charm.MustParseURL("ch:ubuntu"), false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientValidateOriginSuite) TestValidateOriginWithEmptyArch(c *gc.C) {
	origin := corecharm.Origin{
		Source: "charm-hub",
	}

	err := validateOrigin(origin, charm.MustParseURL("ch:ubuntu"), false)
	c.Assert(err, gc.ErrorMatches, "empty architecture not valid")
}

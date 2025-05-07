// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"

	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type clientNormalizeOriginSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&clientNormalizeOriginSuite{})

func (s *clientNormalizeOriginSuite) TestNormalizeCharmOriginNoAll(c *tc.C) {
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
	obtained, err := normalizeCharmOrigin(context.Background(), origin, "amd64", loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	origin.Architecture = "amd64"
	c.Assert(obtained, tc.DeepEquals, origin)
}

func (s *clientNormalizeOriginSuite) TestNormalizeCharmOriginWithEmpty(c *tc.C) {
	track := "1.0"
	origin := params.CharmOrigin{
		Source:       "charm-hub",
		Type:         "charm",
		Risk:         "edge",
		Track:        &track,
		Architecture: "",
		Base:         params.Base{Channel: "all"},
	}
	obtained, err := normalizeCharmOrigin(context.Background(), origin, "amd64", loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	origin.Architecture = "amd64"
	origin.Base.Channel = ""
	c.Assert(obtained, tc.DeepEquals, origin)
}

type clientValidateOriginSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&clientValidateOriginSuite{})

func (s *clientValidateOriginSuite) TestValidateOrigin(c *tc.C) {
	origin := corecharm.Origin{
		Source:   "charm-hub",
		Platform: corecharm.Platform{Architecture: "all"},
	}

	err := validateOrigin(origin, charm.MustParseURL("ch:ubuntu"), false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *clientValidateOriginSuite) TestValidateOriginWithEmptyArch(c *tc.C) {
	origin := corecharm.Origin{
		Source: "charm-hub",
	}

	err := validateOrigin(origin, charm.MustParseURL("ch:ubuntu"), false)
	c.Assert(err, tc.ErrorMatches, "empty architecture not valid")
}

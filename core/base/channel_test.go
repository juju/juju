// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type ChannelSuite struct {
	testhelpers.IsolationSuite
}

func TestChannelSuite(t *testing.T) {
	tc.Run(t, &ChannelSuite{})
}

func (s *ChannelSuite) TestParse(c *tc.C) {
	ch, err := ParseChannel("22.04")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch, tc.DeepEquals, Channel{Track: "22.04"})
	ch, err = ParseChannel("22.04/edge")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch, tc.DeepEquals, Channel{Track: "22.04", Risk: "edge"})
	ch, err = ParseChannel("all")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch, tc.DeepEquals, Channel{Track: "all"})
}

func (s *ChannelSuite) TestParseError(c *tc.C) {
	_, err := ParseChannel("22.04/edge/foo")
	c.Assert(err, tc.ErrorMatches, `channel is malformed and has too many components "22.04/edge/foo"`)
	_, err = ParseChannel("22.04/foo")
	c.Assert(err, tc.ErrorMatches, `risk in channel "22.04/foo" not valid`)
}

func (s *ChannelSuite) TestParseNormalise(c *tc.C) {
	ch, err := ParseChannelNormalize("22.04")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch, tc.DeepEquals, Channel{Track: "22.04", Risk: "stable"})
	ch, err = ParseChannelNormalize("22.04/edge")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch, tc.DeepEquals, Channel{Track: "22.04", Risk: "edge"})
}

func (s *ChannelSuite) TestMakeDefaultChannel(c *tc.C) {
	ch := MakeDefaultChannel("22.04")
	c.Assert(ch, tc.DeepEquals, Channel{Track: "22.04", Risk: "stable"})
}

func (s *ChannelSuite) TestString(c *tc.C) {
	c.Assert(Channel{Track: "22.04"}.String(), tc.Equals, "22.04")
	c.Assert(Channel{Track: "22.04", Risk: "edge"}.String(), tc.Equals, "22.04/edge")
}

func (s *ChannelSuite) TestDisplayString(c *tc.C) {
	c.Assert(Channel{Track: "18.04"}.DisplayString(), tc.Equals, "18.04")
	c.Assert(Channel{Track: "20.04", Risk: "stable"}.DisplayString(), tc.Equals, "20.04")
	c.Assert(Channel{Track: "22.04", Risk: "edge"}.DisplayString(), tc.Equals, "22.04/edge")
}

// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ChannelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ChannelSuite{})

func (s *ChannelSuite) TestParse(c *gc.C) {
	ch, err := ParseChannel("22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, jc.DeepEquals, Channel{Track: "22.04"})
	ch, err = ParseChannel("22.04/edge")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, jc.DeepEquals, Channel{Track: "22.04", Risk: "edge"})
	ch, err = ParseChannel("all")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, jc.DeepEquals, Channel{Track: "all"})
}

func (s *ChannelSuite) TestParseError(c *gc.C) {
	_, err := ParseChannel("22.04/edge/foo")
	c.Assert(err, gc.ErrorMatches, `channel is malformed and has too many components "22.04/edge/foo"`)
	_, err = ParseChannel("22.04/foo")
	c.Assert(err, gc.ErrorMatches, `risk in channel "22.04/foo" not valid`)
}

func (s *ChannelSuite) TestParseNormalise(c *gc.C) {
	ch, err := ParseChannelNormalize("22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, jc.DeepEquals, Channel{Track: "22.04", Risk: "stable"})
	ch, err = ParseChannelNormalize("22.04/edge")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch, jc.DeepEquals, Channel{Track: "22.04", Risk: "edge"})
}

func (s *ChannelSuite) TestMakeDefaultChannel(c *gc.C) {
	ch := MakeDefaultChannel("22.04")
	c.Assert(ch, jc.DeepEquals, Channel{Track: "22.04", Risk: "stable"})
}

func (s *ChannelSuite) TestString(c *gc.C) {
	c.Assert(Channel{Track: "22.04"}.String(), gc.Equals, "22.04")
	c.Assert(Channel{Track: "22.04", Risk: "edge"}.String(), gc.Equals, "22.04/edge")
}

func (s *ChannelSuite) TestDisplayString(c *gc.C) {
	c.Assert(Channel{Track: "18.04"}.DisplayString(), gc.Equals, "18.04")
	c.Assert(Channel{Track: "20.04", Risk: "stable"}.DisplayString(), gc.Equals, "20.04")
	c.Assert(Channel{Track: "22.04", Risk: "edge"}.DisplayString(), gc.Equals, "22.04/edge")
}

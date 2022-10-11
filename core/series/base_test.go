// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type BaseSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BaseSuite{})

func (s *BaseSuite) TestParseBase(c *gc.C) {
	base, err := ParseBase("ubuntu", "22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base, jc.DeepEquals, Base{OS: "ubuntu", Channel: Channel{Track: "22.04", Risk: "stable"}})
	base, err = ParseBase("ubuntu", "22.04/edge")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base, jc.DeepEquals, Base{OS: "ubuntu", Channel: Channel{Track: "22.04", Risk: "edge"}})
}

func (s *BaseSuite) TestGetBaseFromSeries(c *gc.C) {
	base, err := GetBaseFromSeries("jammy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(base, jc.DeepEquals, Base{OS: "ubuntu", Channel: Channel{Track: "22.04", Risk: "stable"}})
}

func (s *BaseSuite) TestGetSeriesFromChannel(c *gc.C) {
	series, err := GetSeriesFromChannel("ubuntu", "22.04")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "jammy")
	series, err = GetSeriesFromChannel("ubuntu", "22.04/edge")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "jammy")
}

func (s *BaseSuite) TestGetSeriesFromBase(c *gc.C) {
	series, err := GetSeriesFromBase(MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "jammy")
}

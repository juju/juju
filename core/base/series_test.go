// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type SeriesSuite struct {
	testing.IsolationSuite
}

func (s *SeriesSuite) TestGetSeriesFromBase(c *gc.C) {
	series, err := GetSeriesFromBase(MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "jammy")
}

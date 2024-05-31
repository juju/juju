// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/base"
)

type SeriesSuite struct {
	testing.IsolationSuite
}

func (s *SeriesSuite) TestGetSeriesFromBase(c *gc.C) {
	series, err := base.GetSeriesFromBase(base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, gc.Equals, "jammy")
}

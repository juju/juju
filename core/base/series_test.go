// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/base"
)

type SeriesSuite struct {
	testing.IsolationSuite
}

func (s *SeriesSuite) TestGetSeriesFromBase(c *tc.C) {
	series, err := base.GetSeriesFromBase(base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(series, tc.Equals, "jammy")
}

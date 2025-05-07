// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/base"
)

type SeriesSuite struct {
	testing.IsolationSuite
}

func (s *SeriesSuite) TestGetSeriesFromBase(c *tc.C) {
	series, err := base.GetSeriesFromBase(base.MakeDefaultBase("ubuntu", "22.04"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(series, tc.Equals, "jammy")
}

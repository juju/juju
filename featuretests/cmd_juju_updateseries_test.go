// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type cmdUpdateSeriesSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdUpdateSeriesSuite) TestUpdateApplicationSeries(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "multi-series", URL: "local:quantal/multi-series-1"})
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: charm})
	_ = s.Factory.MakeUnit(c, &factory.UnitParams{Application: app, SetCharmURL: true})
	c.Assert(app.Series(), gc.Equals, "quantal")
	runCommandExpectSuccess(c, "update-series", "multi-series", "trusty")
	err := app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.Series(), gc.Equals, "trusty")
}

func (s *cmdUpdateSeriesSuite) TestUpdateMachineSeries(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "multi-series", URL: "local:quantal/multi-series-1"})
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: charm})
	_ = s.Factory.MakeUnit(c, &factory.UnitParams{Application: app, SetCharmURL: true})
	runCommandExpectSuccess(c, "update-series", "0", "trusty")
	machine, err := s.State.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Series(), gc.Equals, "trusty")
}

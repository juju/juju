// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/v2/juju/testing"
	"github.com/juju/juju/v2/state"
	"github.com/juju/juju/v2/testing/factory"
)

type cmdSetSeriesSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdSetSeriesSuite) TestSetApplicationSeries(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "multi-series", URL: "local:quantal/multi-series-1"})
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm:       charm,
		CharmOrigin: &state.CharmOrigin{},
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: app, SetCharmURL: true})
	c.Assert(app.Series(), gc.Equals, "quantal")
	context, err := runCommand(c, append([]string{"set-series"}, "multi-series", "trusty")...)
	c.Assert(err, gc.IsNil)
	c.Check(cmdtesting.Stderr(context), jc.Contains, "WARNING To ensure the correct charm binaries are installed when add-unit is next called, please first run `juju refresh` for this application and any related subordinates.")

	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.Series(), gc.Equals, "trusty")
}

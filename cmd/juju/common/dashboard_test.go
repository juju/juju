// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/environs/dashboard"
	"github.com/juju/juju/testing"
)

type dashboardSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&dashboardSuite{})

func (s *dashboardSuite) TestDashboardDataSourceBaseURL(c *gc.C) {
	c.Assert(common.DashboardDataSourceBaseURL(), gc.Equals, dashboard.DefaultBaseURL)
	url := "https://1.2.3.4/streams/dashboard"
	s.PatchEnvironment("JUJU_DASHBOARD_SIMPLESTREAMS_URL", url)
	c.Assert(common.DashboardDataSourceBaseURL(), gc.Equals, url)
}

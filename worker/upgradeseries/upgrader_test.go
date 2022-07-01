// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/paths"
	"github.com/juju/juju/v3/service/systemd"
	"github.com/juju/juju/v3/testing"
	"github.com/juju/juju/v3/worker/upgradeseries"
	. "github.com/juju/juju/v3/worker/upgradeseries/mocks"
)

type upgraderSuite struct {
	testing.BaseSuite

	machineService string

	logger  upgradeseries.Logger
	manager *MockSystemdServiceManager
}

var _ = gc.Suite(&upgraderSuite{})

func (s *upgraderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.logger = loggo.GetLogger("test.upgrader")
	s.machineService = "jujud-machine-0"
}

func (s *upgraderSuite) TestToSystemdServicesWritten(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	s.patchFrom("trusty")

	s.manager.EXPECT().WriteSystemdAgent(
		s.machineService, paths.NixDataDir, systemd.EtcSystemdMultiUserDir,
	).Return(nil)

	upg := s.newUpgrader(c, "trusty", "xenial")
	c.Assert(upg.PerformUpgrade(), jc.ErrorIsNil)
}

func (s *upgraderSuite) setupMocks(ctrl *gomock.Controller) {
	s.manager = NewMockSystemdServiceManager(ctrl)

	// FindAgents would look through the dataDir to find agents.
	// Here we return unit agents, but they should have no impact on methods.
	s.manager.EXPECT().FindAgents(paths.NixDataDir).Return(s.machineService, []string{"jujud-unit-ubuntu-0", "jujud-unit-redis-0"}, nil, nil)
}

func (s *upgraderSuite) newUpgrader(c *gc.C, currentSeries, toSeries string) upgradeseries.Upgrader {
	upg, err := upgradeseries.NewUpgrader(currentSeries, toSeries, s.manager, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	return upg
}

func (s *upgraderSuite) patchFrom(series string) {
	upgradeseries.PatchHostSeries(s, series)
}

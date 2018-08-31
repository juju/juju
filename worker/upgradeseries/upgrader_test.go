// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/service/systemd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/upgradeseries"
	. "github.com/juju/juju/worker/upgradeseries/mocks"
)

type upgraderSuite struct {
	testing.BaseSuite

	machineService string
	unitServices   []string

	logger  upgradeseries.Logger
	manager *MockSystemdServiceManager
}

var _ = gc.Suite(&upgraderSuite{})

func (s *upgraderSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.machineService = "jujud-machine-0"
	s.unitServices = []string{"jujud-unit-ubuntu-0", "jujud-unit-redis-0"}
}

func (s *upgraderSuite) TestNotToSystemdNoAction(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	s.patchFrom("precise")

	// No expectations.

	upg := s.newUpgrader(c, "trusty")
	c.Assert(upg.PerformUpgrade(), jc.ErrorIsNil)
}

func (s *upgraderSuite) TestFromSystemdNoAction(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	s.patchFrom("xenial")

	// No expectations.

	upg := s.newUpgrader(c, "bionic")
	c.Assert(upg.PerformUpgrade(), jc.ErrorIsNil)
}

func (s *upgraderSuite) TestToSystemdServicesWritten(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	s.patchFrom("trusty")

	s.manager.EXPECT().WriteSystemdAgents(
		s.machineService, s.unitServices, paths.NixDataDir, systemd.EtcSystemdDir, systemd.EtcSystemdMultiUserDir,
	).Return(append(s.unitServices, s.machineService), nil, nil, nil)

	upg := s.newUpgrader(c, "xenial")
	c.Assert(upg.PerformUpgrade(), jc.ErrorIsNil)
}

func (s *upgraderSuite) setupMocks(ctrl *gomock.Controller) {
	s.logger = voidLogger(ctrl)
	s.manager = NewMockSystemdServiceManager(ctrl)

	s.manager.EXPECT().FindAgents(paths.NixDataDir).Return(s.machineService, s.unitServices, nil, nil)
}

func (s *upgraderSuite) newUpgrader(c *gc.C, toSeries string) upgradeseries.Upgrader {
	upg, err := upgradeseries.NewUpgrader(toSeries, s.manager, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	return upg
}

func (s *upgraderSuite) patchFrom(series string) {
	upgradeseries.PatchHostSeries(s, series)
}

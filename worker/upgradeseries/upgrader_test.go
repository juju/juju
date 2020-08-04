// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
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

func (s *upgraderSuite) TestNotToSystemdCopyToolsOnly(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	s.patchFrom("precise")

	// No systemd file changes; just the new tools for the target series.
	s.manager.EXPECT().CopyAgentBinary(
		s.machineService, paths.NixDataDir, "trusty", "precise", version.Current,
	).Return(nil)

	upg := s.newUpgrader(c, "precise", "trusty")
	c.Assert(upg.PerformUpgrade(), jc.ErrorIsNil)
}

func (s *upgraderSuite) TestFromSystemdCopyToolsOnly(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	s.patchFrom("xenial")

	// No systemd file changes; just the new tools for the target series.
	s.manager.EXPECT().CopyAgentBinary(
		s.machineService, paths.NixDataDir, "bionic", "xenial", version.Current,
	).Return(nil)

	upg := s.newUpgrader(c, "xenial", "bionic")
	c.Assert(upg.PerformUpgrade(), jc.ErrorIsNil)
}

func (s *upgraderSuite) TestFromSystemdCopyToolsForAlreadyUpgradedMachine(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	// Actual series is Bionic.
	s.patchFrom("bionic")

	// No systemd file changes; just the new tools for the target series.
	s.manager.EXPECT().CopyAgentBinary(
		s.machineService, paths.NixDataDir, "bionic", "xenial", version.Current,
	).Return(nil)

	// Juju thinks the machine is Xenial.
	upg := s.newUpgrader(c, "xenial", "bionic")
	c.Assert(upg.PerformUpgrade(), jc.ErrorIsNil)
}

func (s *upgraderSuite) TestToSystemdServicesWritten(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupMocks(ctrl)

	s.patchFrom("trusty")

	s.manager.EXPECT().WriteSystemdAgents(
		s.machineService, paths.NixDataDir, systemd.EtcSystemdMultiUserDir,
	).Return(nil)

	s.manager.EXPECT().CopyAgentBinary(
		s.machineService, paths.NixDataDir, "xenial", "trusty", version.Current,
	).Return(nil)

	upg := s.newUpgrader(c, "trusty", "xenial")
	c.Assert(upg.PerformUpgrade(), jc.ErrorIsNil)
}

func (s *upgraderSuite) setupMocks(ctrl *gomock.Controller) {
	s.logger = voidLogger(ctrl)
	s.manager = NewMockSystemdServiceManager(ctrl)

	s.manager.EXPECT().FindAgents(paths.NixDataDir).Return(s.machineService, s.unitServices, nil, nil)
}

func (s *upgraderSuite) newUpgrader(c *gc.C, currentSeries, toSeries string) upgradeseries.Upgrader {
	upg, err := upgradeseries.NewUpgrader(currentSeries, toSeries, s.manager, s.logger)
	c.Assert(err, jc.ErrorIsNil)
	return upg
}

func (s *upgraderSuite) patchFrom(series string) {
	upgradeseries.PatchHostSeries(s, series)
}

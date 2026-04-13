// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/semversion"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
	"github.com/juju/juju/internal/worker/upgradedatabase/upgradesteps"
)

type manifoldSuite struct {
	baseSuite

	upgradeServices       *MockUpgradeServices
	upgradeServicesGetter *MockUpgradeServicesGetter
	worker                *MockWorker
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.upgradeServicesGetter = NewMockUpgradeServicesGetter(ctrl)
	s.upgradeServices = NewMockUpgradeServices(ctrl)
	s.worker = NewMockWorker(ctrl)

	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig).AnyTimes()
	s.agentConfig.EXPECT().Tag().Return(names.NewMachineTag("0")).AnyTimes()
	s.agentConfig.EXPECT().UpgradedToVersion().Return(semversion.MustParse("1.0.0")).AnyTimes()
	s.upgradeServices.EXPECT().Upgrade().Return(
		&upgradeservice.WatchableService{},
	).AnyTimes()
	s.upgradeServices.EXPECT().ControllerNode().Return(
		&controllernodeservice.Service{},
	).AnyTimes()
	s.upgradeServicesGetter.EXPECT().ServicesForController().Return(
		s.upgradeServices,
	).AnyTimes()

	c.Cleanup(func() {
		s.upgradeServices = nil
		s.worker = nil
	})
	return ctrl
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.AgentName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.UpgradeDBGateName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.UpgradeServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.DBAccessorName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.UpgradeSteps = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestVersionWindowIncludes(c *tc.C) {
	window := VersionWindow{
		From: semversion.MustParse("4.0.0"),
		To:   semversion.MustParse("4.0.10"),
	}

	c.Check(window.Includes(semversion.MustParse("4.0.0")), tc.Equals, true)
	c.Check(window.Includes(semversion.MustParse("4.0.5")), tc.Equals, true)
	c.Check(window.Includes(semversion.MustParse("4.0.9")), tc.Equals, true)

	c.Check(window.Includes(semversion.MustParse("3.9.9")), tc.Equals, false)
	c.Check(window.Includes(semversion.MustParse("4.0.10")), tc.Equals, false)
	c.Check(window.Includes(semversion.MustParse("4.1.0")), tc.Equals, false)
}

func (s *manifoldSuite) TestFilterSteps(c *tc.C) {
	allSteps := map[VersionWindow][]UpgradeStep{
		window_4_0_0_to_4_0_1: {
			upgradesteps.Step0001_PatchModelConfigCloudType,
		},
	}

	steps := filterSteps(allSteps, semversion.MustParse("4.0.0"))
	c.Check(steps, tc.HasLen, 1)

	steps = filterSteps(allSteps, semversion.MustParse("4.0.5"))
	c.Check(steps, tc.HasLen, 0)

	steps = filterSteps(allSteps, semversion.MustParse("4.0.10"))
	c.Check(steps, tc.HasLen, 0)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:           "agent",
		UpgradeDBGateName:   "upgrade-database-lock",
		UpgradeServicesName: "upgrade-services",
		DBAccessorName:      "db-accessor",
		Logger:              s.logger,
		Clock:               clock.WallClock,
		NewWorker:           func(Config) (worker.Worker, error) { return s.worker, nil },
		UpgradeSteps:        map[VersionWindow][]UpgradeStep{},
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"agent":                 s.agent,
		"upgrade-database-lock": s.lock,
		"upgrade-services":      s.upgradeServicesGetter,
		"db-accessor":           s.dbGetter,
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{
	"agent", "upgrade-database-lock", "upgrade-services", "db-accessor",
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWorker()

	w, err := Manifold(s.getConfig()).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) expectWorker() {
	s.worker.EXPECT().Kill()
	s.worker.EXPECT().Wait().Return(nil)
}

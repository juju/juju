// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/semversion"
	controllernodeservice "github.com/juju/juju/domain/controllernode/service"
	modelservice "github.com/juju/juju/domain/model/service"
	upgradeservice "github.com/juju/juju/domain/upgrade/service"
)

type manifoldSuite struct {
	baseSuite

	worker *MockWorker
}

func TestManifoldSuite(t *stdtesting.T) { tc.Run(t, &manifoldSuite{}) }
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
	cfg.DomainServicesName = ""
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
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:          "agent",
		UpgradeDBGateName:  "upgrade-database-lock",
		DomainServicesName: "domain-services",
		DBAccessorName:     "db-accessor",
		Logger:             s.logger,
		Clock:              clock.WallClock,
		NewWorker:          func(Config) (worker.Worker, error) { return s.worker, nil },
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"agent":                 s.agent,
		"upgrade-database-lock": s.lock,
		"domain-services":       s.domainServices,
		"db-accessor":           s.dbGetter,
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"agent", "upgrade-database-lock", "domain-services", "db-accessor"}

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

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.worker = NewMockWorker(ctrl)

	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig).AnyTimes()
	s.agentConfig.EXPECT().Tag().Return(names.NewMachineTag("0")).AnyTimes()
	s.agentConfig.EXPECT().UpgradedToVersion().Return(semversion.MustParse("1.0.0")).AnyTimes()

	s.domainServices.EXPECT().Upgrade().Return(&upgradeservice.WatchableService{}).AnyTimes()
	s.domainServices.EXPECT().Model().Return(&modelservice.WatchableService{}).AnyTimes()
	s.domainServices.EXPECT().ControllerNode().Return(&controllernodeservice.WatchableService{}).AnyTimes()

	return ctrl
}

func (s *manifoldSuite) expectWorker() {
	s.worker.EXPECT().Kill()
	s.worker.EXPECT().Wait().Return(nil)
}

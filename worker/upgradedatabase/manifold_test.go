// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dependencytesting "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	upgradeservice "github.com/juju/juju/domain/upgrade/service"
)

type manifoldSuite struct {
	baseSuite

	worker *MockWorker
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.AgentName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.UpgradeDBGateName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ServiceFactoryName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:          "agent",
		UpgradeDBGateName:  "upgrade-database-lock",
		ServiceFactoryName: "service-factory",
		DBAccessorName:     "db-accessor",
		Logger:             s.logger,
		NewWorker:          func(Config) (worker.Worker, error) { return s.worker, nil },
	}
}

func (s *manifoldSuite) getContext() dependency.Context {
	resources := map[string]any{
		"agent":                 s.agent,
		"upgrade-database-lock": s.lock,
		"service-factory":       s.serviceFactory,
		"db-accessor":           s.dbGetter,
	}
	return dependencytesting.StubContext(nil, resources)
}

var expectedInputs = []string{"agent", "upgrade-database-lock", "service-factory", "db-accessor"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWorker()

	w, err := Manifold(s.getConfig()).Start(s.getContext())
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.worker = NewMockWorker(ctrl)

	s.serviceFactory.EXPECT().Upgrade().Return(&upgradeservice.Service{}).AnyTimes()

	return ctrl
}

func (s *manifoldSuite) expectWorker() {
	s.worker.EXPECT().Kill()
	s.worker.EXPECT().Wait().Return(nil)
}

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/dependency"
	dependencytesting "github.com/juju/worker/v3/dependency/testing"
	gc "gopkg.in/check.v1"

	state "github.com/juju/juju/state"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.AgentName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.StateName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:         "agent",
		ObjectStoreName:   "object-store",
		StateName:         "state",
		BootstrapGateName: "bootstrap-gate",
		Logger:            s.logger,
		AgentBinarySeeder: func() error { return nil },
		RequiresBootstrap: func(appService ApplicationService) (bool, error) { return false, nil },
	}
}

func (s *manifoldSuite) getContext() dependency.Context {
	resources := map[string]any{
		"agent":          s.agent,
		"state":          s.stateTracker,
		"object-store":   s.objectStore,
		"bootstrap-gate": s.bootstrapUnlocker,
	}
	return dependencytesting.StubContext(nil, resources)
}

var expectedInputs = []string{"agent", "state", "object-store", "bootstrap-gate"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStartAlreadyBootstrapped(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectStateTracker()
	s.expectGateUnlock()

	_, err := Manifold(s.getConfig()).Start(s.getContext())
	c.Assert(err, jc.ErrorIs, dependency.ErrUninstall)
}

func (s *manifoldSuite) expectStateTracker() {
	s.stateTracker.EXPECT().Use().Return(&state.StatePool{}, &state.State{}, nil)
	s.stateTracker.EXPECT().Done()
}

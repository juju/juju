// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	dt "github.com/juju/worker/v3/dependency/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/worker/sshserver"
	"github.com/juju/juju/worker/sshserver/mocks"
)

type manifoldSuite struct {
}

var _ = gc.Suite(&manifoldSuite{})

func newManifoldConfig(l *mocks.MockLogger, modifier func(cfg *sshserver.ManifoldConfig)) *sshserver.ManifoldConfig {
	cfg := &sshserver.ManifoldConfig{
		StateName:              "state",
		AgentName:              "agent",
		NewServerWrapperWorker: func(sshserver.ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func() (worker.Worker, error) { return nil, nil },
		Logger:                 l,
	}

	modifier(cfg)

	return cfg
}

func (s *manifoldSuite) TestConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)

	// Check config as expected.

	cfg := newManifoldConfig(mockLogger, func(cfg *sshserver.ManifoldConfig) {})
	c.Assert(cfg.Validate(), gc.IsNil)

	// Entirely missing.
	cfg = newManifoldConfig(mockLogger, func(cfg *sshserver.ManifoldConfig) {
		cfg.StateName = ""
		cfg.AgentName = ""
		cfg.NewServerWrapperWorker = nil
		cfg.NewServerWorker = nil
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing state name.
	cfg = newManifoldConfig(mockLogger, func(cfg *sshserver.ManifoldConfig) {
		cfg.StateName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing agent name.
	cfg = newManifoldConfig(mockLogger, func(cfg *sshserver.ManifoldConfig) {
		cfg.AgentName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWrapperWorker.
	cfg = newManifoldConfig(mockLogger, func(cfg *sshserver.ManifoldConfig) {
		cfg.NewServerWrapperWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWorker.
	cfg = newManifoldConfig(mockLogger, func(cfg *sshserver.ManifoldConfig) {
		cfg.NewServerWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing Logger.
	cfg = newManifoldConfig(mockLogger, func(cfg *sshserver.ManifoldConfig) {
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

}

func (s *manifoldSuite) TestManifoldStart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockAgent := mocks.NewMockAgent(ctrl)
	mockAgentConfig := mocks.NewMockConfig(ctrl)
	mockState := mocks.NewMockStateTracker(ctrl)

	mockAgent.EXPECT().CurrentConfig().Return(mockAgentConfig).AnyTimes()
	mockAgentConfig.EXPECT().StateServingInfo().Return(controller.StateServingInfo{}, true).Times(1)
	mockState.EXPECT().Use().Times(1)

	manifold := sshserver.Manifold(sshserver.ManifoldConfig{
		AgentName:              "agent-name",
		StateName:              "state",
		NewServerWrapperWorker: func(sshserver.ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func() (worker.Worker, error) { return nil, nil },
		Logger:                 mockLogger,
	})

	worker, err := manifold.Start(
		dt.StubContext(nil, map[string]interface{}{
			"state":      mockState,
			"agent-name": mockAgent,
		}),
	)
	c.Assert(err, gc.IsNil)
	c.Assert(worker, gc.NotNil)
}

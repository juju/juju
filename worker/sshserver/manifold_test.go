// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	dt "github.com/juju/worker/v3/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	dbtesting "github.com/juju/juju/database/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/sshserver"
	"github.com/juju/testing"
)

type manifoldSuite struct {
	dbtesting.ControllerSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestConfigValidate(c *gc.C) {
	// Check config as expected.
	cfg := sshserver.ManifoldConfig{
		StateName:              "state",
		AgentName:              "agent",
		NewServerWrapperWorker: func(sshserver.ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func() (*sshserver.ServerWorker, error) { return nil, nil },
		Logger:                 stubLogger{},
	}

	c.Assert(cfg.Validate(), gc.IsNil)

	// Entirely missing.
	cfg = sshserver.ManifoldConfig{}
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing state name.
	cfg = sshserver.ManifoldConfig{
		AgentName:              "agent",
		NewServerWrapperWorker: func(sshserver.ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func() (*sshserver.ServerWorker, error) { return nil, nil },
		Logger:                 stubLogger{},
	}
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing agent name.
	cfg = sshserver.ManifoldConfig{
		StateName:              "state",
		NewServerWrapperWorker: func(sshserver.ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func() (*sshserver.ServerWorker, error) { return nil, nil },
		Logger:                 stubLogger{},
	}
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWrapperWorker.
	cfg = sshserver.ManifoldConfig{
		StateName:       "state",
		AgentName:       "agent",
		NewServerWorker: func() (*sshserver.ServerWorker, error) { return nil, nil },
		Logger:          stubLogger{},
	}
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWorker.
	cfg = sshserver.ManifoldConfig{
		StateName:              "state",
		AgentName:              "agent",
		NewServerWrapperWorker: func(sshserver.ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		Logger:                 stubLogger{},
	}
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing Logger.
	cfg = sshserver.ManifoldConfig{
		StateName:              "state",
		AgentName:              "agent",
		NewServerWrapperWorker: func(sshserver.ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func() (*sshserver.ServerWorker, error) { return nil, nil },
	}
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

}

func (s *manifoldSuite) TestManifoldStart(c *gc.C) {
	manifold := sshserver.Manifold(sshserver.ManifoldConfig{
		AgentName:              "agent-name",
		StateName:              "state",
		NewServerWrapperWorker: func(sshserver.ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func() (*sshserver.ServerWorker, error) { return nil, nil },
		Logger:                 stubLogger{},
	})

	stateTracker := &stubStateTracker{}
	agentConf := &stubAgentConfig{}
	agent := &stubAgent{
		conf: agentConf,
	}
	worker, err := manifold.Start(
		dt.StubContext(nil, map[string]interface{}{
			"state":      stateTracker,
			"agent-name": agent,
		}),
	)
	c.Assert(err, gc.IsNil)
	c.Assert(worker, gc.NotNil)

	stateTracker.CheckCallNames(c, "Use")
	agent.CheckCallNames(c, "CurrentConfig")
	agentConf.CheckCallNames(c, "StateServingInfo")
}

type stubStateTracker struct {
	testing.Stub
	pool state.StatePool
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return &s.pool, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}

func (s *stubStateTracker) Report() map[string]interface{} {
	s.MethodCall(s, "Report")
	return nil
}

type stubAgent struct {
	testing.Stub
	agent.Agent
	conf *stubAgentConfig
}

func (sa *stubAgent) CurrentConfig() agent.Config {
	sa.MethodCall(sa, "CurrentConfig")
	return sa.conf
}

type stubAgentConfig struct {
	testing.Stub
	agent.ConfigSetter
}

func (sac *stubAgentConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	sac.MethodCall(sac, "StateServingInfo")
	return controller.StateServingInfo{}, true
}

type stubLogger struct{}

func (stubLogger) Errorf(string, ...interface{}) {}

// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerport_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/controllerport"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	config           controllerport.ManifoldConfig
	manifold         dependency.Manifold
	context          dependency.Context
	agent            *mockAgent
	hub              *pubsub.StructuredHub
	state            stubStateTracker
	logger           loggo.Logger
	controllerConfig controller.Config
	worker           worker.Worker

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = &mockAgent{}
	s.hub = pubsub.NewStructuredHub(nil)
	s.state = stubStateTracker{}
	s.logger = loggo.GetLogger("controllerport_manifold")
	s.controllerConfig = controller.Config(map[string]interface{}{
		"controller-api-port": 2048,
	})
	s.worker = &struct{ worker.Worker }{}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.config = controllerport.ManifoldConfig{
		AgentName:               "agent",
		HubName:                 "hub",
		StateName:               "state",
		Logger:                  s.logger,
		UpdateControllerAPIPort: s.updatePort,
		GetControllerConfig:     s.getControllerConfig,
		NewWorker:               s.newWorker,
	}
	s.manifold = controllerport.Manifold(s.config)
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent": s.agent,
		"hub":   s.hub,
		"state": &s.state,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) getControllerConfig(st *state.State) (controller.Config, error) {
	s.stub.MethodCall(s, "GetControllerConfig", st)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.controllerConfig, nil
}

func (s *ManifoldSuite) updatePort(port int) error {
	return errors.Errorf("braincake")
}

func (s *ManifoldSuite) newWorker(config controllerport.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

var expectedInputs = []string{"state", "agent", "hub"}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestValidate(c *gc.C) {
	type test struct {
		f      func(*controllerport.ManifoldConfig)
		expect string
	}
	tests := []test{{
		func(cfg *controllerport.ManifoldConfig) { cfg.StateName = "" },
		"empty StateName not valid",
	}, {
		func(cfg *controllerport.ManifoldConfig) { cfg.HubName = "" },
		"empty HubName not valid",
	}, {
		func(cfg *controllerport.ManifoldConfig) { cfg.AgentName = "" },
		"empty AgentName not valid",
	}, {
		func(cfg *controllerport.ManifoldConfig) { cfg.Logger = nil },
		"nil Logger not valid",
	}, {
		func(cfg *controllerport.ManifoldConfig) { cfg.UpdateControllerAPIPort = nil },
		"nil UpdateControllerAPIPort not valid",
	}, {
		func(cfg *controllerport.ManifoldConfig) { cfg.GetControllerConfig = nil },
		"nil GetControllerConfig not valid",
	}, {
		func(cfg *controllerport.ManifoldConfig) { cfg.NewWorker = nil },
		"nil NewWorker not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		config := s.config
		test.f(&config)
		manifold := controllerport.Manifold(config)
		w, err := manifold.Start(s.context)
		workertest.CheckNilOrKill(c, w)
		c.Check(err, gc.ErrorMatches, test.expect)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	s.startWorkerClean(c)

	s.stub.CheckCallNames(c, "GetControllerConfig", "NewWorker")
	args := s.stub.Calls()[1].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, controllerport.Config{})
	config := args[0].(controllerport.Config)

	// Can't directly compare functions, so blank it out.
	c.Assert(config.UpdateControllerAPIPort(3), gc.ErrorMatches, "braincake")
	config.UpdateControllerAPIPort = nil

	c.Assert(config, jc.DeepEquals, controllerport.Config{
		AgentConfig:       &s.agent.conf,
		Hub:               s.hub,
		Logger:            s.logger,
		ControllerAPIPort: 2048,
	})
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, s.worker)
	return w
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

type mockAgent struct {
	agent.Agent
	conf mockAgentConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

type mockAgentConfig struct {
	agent.Config
	port int
}

func (c *mockAgentConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	return controller.StateServingInfo{ControllerAPIPort: c.port}, true
}

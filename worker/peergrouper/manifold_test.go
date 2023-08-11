// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker/peergrouper"
	"github.com/juju/juju/worker/servicefactory"
)

type ManifoldSuite struct {
	statetesting.StateSuite

	manifold               dependency.Manifold
	context                dependency.Context
	clock                  *testclock.Clock
	agent                  *mockAgent
	hub                    *mockHub
	registerer             *fakeRegisterer
	stateTracker           stubStateTracker
	serviceFactory         servicefactory.ServiceFactory
	controllerConfigGetter *controllerconfigservice.Service

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.clock = testclock.NewClock(time.Time{})
	s.agent = &mockAgent{conf: mockAgentConfig{
		info: &controller.StateServingInfo{
			StatePort: 1234,
			APIPort:   5678,
		},
	}}
	s.hub = &mockHub{}
	s.registerer = &fakeRegisterer{}
	s.stateTracker = stubStateTracker{pool: s.StatePool, state: s.State}
	s.controllerConfigGetter = &controllerconfigservice.Service{}
	s.serviceFactory = stubServiceFactory{
		controllerConfigGetter: s.controllerConfigGetter,
	}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = peergrouper.Manifold(peergrouper.ManifoldConfig{
		AgentName:            "agent",
		ClockName:            "clock",
		StateName:            "state",
		ServiceFactoryName:   "service-factory",
		Hub:                  s.hub,
		NewWorker:            s.newWorker,
		PrometheusRegisterer: s.registerer,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":           s.agent,
		"clock":           s.clock,
		"state":           &s.stateTracker,
		"service-factory": s.serviceFactory,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config peergrouper.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	w := worker.NewRunner(worker.RunnerParams{})
	s.AddCleanup(func(c *gc.C) { workertest.CleanKill(c, w) })
	return w, nil
}

var expectedInputs = []string{"agent", "clock", "state", "service-factory"}

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

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, peergrouper.Config{})
	config := args[0].(peergrouper.Config)

	c.Assert(config.ControllerId(), gc.Equals, "10")
	config.ControllerId = nil
	c.Assert(config, jc.DeepEquals, peergrouper.Config{
		State:        peergrouper.StateShim{State: s.State},
		MongoSession: peergrouper.MongoSessionShim{Session: s.State.MongoSession()},
		APIHostPortsSetter: &peergrouper.CachingAPIHostPortsSetter{
			APIHostPortsSetter: s.State,
		},
		Clock:                  s.clock,
		Hub:                    s.hub,
		MongoPort:              1234,
		APIPort:                5678,
		SupportsHA:             true,
		PrometheusRegisterer:   s.registerer,
		ControllerConfigGetter: s.controllerConfigGetter,
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	s.stateTracker.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}

func (s *ManifoldSuite) TestNoStateServingInfoClosesState(c *gc.C) {
	s.agent.conf.info = nil

	_, err := s.manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "state serving info missing from agent config")

	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

type stubStateTracker struct {
	testing.Stub
	pool  *state.StatePool
	state *state.State
}

func (s *stubStateTracker) Use() (*state.StatePool, *state.State, error) {
	s.MethodCall(s, "Use")
	return s.pool, s.state, s.NextErr()
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
	info *controller.StateServingInfo
}

func (c *mockAgentConfig) Tag() names.Tag {
	return names.NewMachineTag("10")
}

func (c *mockAgentConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	if c.info != nil {
		return *c.info, true
	}
	return controller.StateServingInfo{}, false
}

type mockHub struct {
	peergrouper.Hub
}

type fakeRegisterer struct {
	prometheus.Registerer
}

type stubServiceFactory struct {
	servicefactory.ServiceFactory
	controllerConfigGetter *controllerconfigservice.Service
}

func (s stubServiceFactory) ControllerConfig() *controllerconfigservice.Service {
	return s.controllerConfigGetter
}

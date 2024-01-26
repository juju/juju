// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/worker/peergrouper"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type ManifoldSuite struct {
	statetesting.StateSuite

	manifold     dependency.Manifold
	getter       dependency.Getter
	clock        *testclock.Clock
	agent        *mockAgent
	hub          *mockHub
	registerer   *fakeRegisterer
	stateTracker stubStateTracker

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.ControllerConfig = map[string]any{
		controller.APIPort: 5678,
	}
	s.StateSuite.SetUpTest(c)

	s.clock = testclock.NewClock(time.Time{})
	s.agent = &mockAgent{conf: mockAgentConfig{}}
	s.hub = &mockHub{}
	s.registerer = &fakeRegisterer{}
	s.stateTracker = stubStateTracker{pool: s.StatePool, state: s.State}
	s.stub.ResetCalls()

	s.getter = s.newGetter(nil)
	s.manifold = peergrouper.Manifold(peergrouper.ManifoldConfig{
		AgentName:            "agent",
		ClockName:            "clock",
		StateName:            "state",
		Hub:                  s.hub,
		NewWorker:            s.newWorker,
		PrometheusRegisterer: s.registerer,
	})
}

func (s *ManifoldSuite) newGetter(overlay map[string]interface{}) dependency.Getter {
	resources := map[string]interface{}{
		"agent": s.agent,
		"clock": s.clock,
		"state": &s.stateTracker,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubGetter(resources)
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

var expectedInputs = []string{"agent", "clock", "state"}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		getter := s.newGetter(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context.Background(), getter)
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
		Clock:                s.clock,
		Hub:                  s.hub,
		MongoPort:            1234,
		APIPort:              5678,
		SupportsHA:           true,
		PrometheusRegisterer: s.registerer,
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
	w, err := s.manifold.Start(context.Background(), s.getter)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
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

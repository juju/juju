// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker/peergrouper"
)

type ManifoldSuite struct {
	statetesting.StateSuite

	manifold     dependency.Manifold
	context      dependency.Context
	clock        *testclock.Clock
	agent        *mockAgent
	hub          *mockHub
	stateTracker stubStateTracker

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.clock = testclock.NewClock(time.Time{})
	s.agent = &mockAgent{conf: mockAgentConfig{
		info: &params.StateServingInfo{
			StatePort: 1234,
			APIPort:   5678,
		},
	}}
	s.hub = &mockHub{}
	s.stateTracker = stubStateTracker{pool: s.StatePool}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = peergrouper.Manifold(peergrouper.ManifoldConfig{
		AgentName:          "agent",
		ClockName:          "clock",
		ControllerPortName: "controller-port",
		StateName:          "state",
		Hub:                s.hub,
		NewWorker:          s.newWorker,
		ControllerSupportsSpaces: func(st *state.State) (bool, error) {
			if st != s.State {
				return false, errors.New("invalid state")
			}
			return true, nil
		},
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":           s.agent,
		"clock":           s.clock,
		"controller-port": nil,
		"state":           &s.stateTracker,
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

var expectedInputs = []string{"agent", "clock", "controller-port", "state"}

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

	c.Assert(config, jc.DeepEquals, peergrouper.Config{
		State:        peergrouper.StateShim{s.State},
		MongoSession: peergrouper.MongoSessionShim{s.State.MongoSession()},
		APIHostPortsSetter: &peergrouper.CachingAPIHostPortsSetter{
			APIHostPortsSetter: s.State,
		},
		Clock:          s.clock,
		Hub:            s.hub,
		SupportsSpaces: true,
		MongoPort:      1234,
		APIPort:        5678,
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

type stubStateTracker struct {
	testing.Stub
	pool *state.StatePool
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return s.pool, s.NextErr()
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
	info *params.StateServingInfo
}

func (c *mockAgentConfig) StateServingInfo() (params.StateServingInfo, bool) {
	if c.info != nil {
		return *c.info, true
	}
	return params.StateServingInfo{}, false
}

type mockHub struct {
	peergrouper.Hub
}

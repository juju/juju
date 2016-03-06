// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/apiserver"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	workerstate "github.com/juju/juju/worker/state"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	newCalled bool
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.newCalled = false
	s.PatchValue(&apiserver.NewWorker,
		func(st *state.State, newApiserverWorker func(st *state.State, certChanged chan params.StateServingInfo) (worker.Worker, error), certChanged chan params.StateServingInfo) (worker.Worker, error) {
			s.newCalled = true
			return new(mockWorker), nil
		},
	)
}

func (s *ManifoldSuite) TestMachineShouldWrite(c *gc.C) {
	config := s.fakeManifoldConfig()
	_, err := runManifold(
		apiserver.Manifold(config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		nil)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
}

func (s *ManifoldSuite) TestMachineShouldntWrite(c *gc.C) {
	config := s.fakeManifoldConfig()
	_, err := runManifold(
		apiserver.Manifold(config),
		&fakeAgent{tag: names.NewMachineTag("42")},
		nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
}

func (s *ManifoldSuite) TestUnit(c *gc.C) {
	config := s.fakeManifoldConfig()
	_, err := runManifold(
		apiserver.Manifold(config),
		&fakeAgent{tag: names.NewUnitTag("foo/0")},
		nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
}

func (s *ManifoldSuite) TestNonAgent(c *gc.C) {
	config := s.fakeManifoldConfig()
	_, err := runManifold(
		apiserver.Manifold(config),
		&fakeAgent{tag: names.NewUserTag("foo")},
		nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.newCalled, jc.IsTrue)
}

type fakeAgent struct {
	agent.Agent
	tag names.Tag
}

func (a *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{tag: a.tag}
}

type fakeConfig struct {
	agent.Config
	tag names.Tag
}

func (c *fakeConfig) Tag() names.Tag {
	return c.tag
}

func (s *ManifoldSuite) fakeManifoldConfig() apiserver.ManifoldConfig {
	return apiserver.ManifoldConfig{
		AgentName: "agent-name",
		StateName: "state",
		NewApiserverWorker: func(st *state.State, certChanged chan params.StateServingInfo) (worker.Worker, error) {
			s.newCalled = true
			return nil, nil
		},
	}
}

func runManifold(
	manifold dependency.Manifold, agent agent.Agent, apiCaller base.APICaller,
) (worker.Worker, error) {
	if agent == nil {
		agent = new(dummyAgent)
	}
	if apiCaller == nil {
		apiCaller = basetesting.APICallerFunc(
			func(string, int, string, string, interface{}, interface{}) error {
				return nil
			})
	}

	// stTracker := workerstate.StateTracker{}
	getResource := dt.StubGetResource(dt.StubResources{
		"upgradewaiter-name": dt.StubResource{Output: true},
		"agent-name":         dt.StubResource{Output: agent},
		"state":              dt.StubResource{Output: new(mockTracker)},
		"api-caller-name":    dt.StubResource{Output: apiCaller},
	})
	return manifold.Start(getResource)
}

type dummyAgent struct {
	agent.Agent
}

type mockTracker struct {
	workerstate.StateTracker
}

func (m *mockTracker) Done() error {
	return nil
}

func (m *mockTracker) Use() (*state.State, error) {
	return nil, nil
}

type mockWorker struct {
	tomb tomb.Tomb
}

func (w *mockWorker) Kill() {
	w.tomb.Kill(nil)
	w.tomb.Done()
}

func (w *mockWorker) Wait() error {
	return w.tomb.Wait()
}

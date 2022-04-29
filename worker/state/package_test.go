// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

var NewStateTracker = newStateTracker

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type BaseSuite struct {
	statetesting.StateSuite

	Manifold        dependency.Manifold
	Resources       dt.StubResources
	Config          ManifoldConfig
	OpenStateErr    error
	OpenStateCalled bool

	agent             *mockAgent
	setStatePoolCalls []*state.StatePool
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.OpenStateCalled = false
	s.OpenStateErr = nil
	s.setStatePoolCalls = nil

	s.Config = ManifoldConfig{
		AgentName:              "agent",
		StateConfigWatcherName: "state-config-watcher",
		OpenStatePool:          s.fakeOpenState,
		SetStatePool: func(pool *state.StatePool) {
			s.setStatePoolCalls = append(s.setStatePoolCalls, pool)
		},
	}
	s.Manifold = Manifold(s.Config)
	s.Resources = dt.StubResources{
		"agent":                dt.NewStubResource(new(mockAgent)),
		"state-config-watcher": dt.NewStubResource(true),
	}
}

func (s *BaseSuite) MustStartManifold(c *gc.C) worker.Worker {
	w, err := s.StartManifold(c)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *BaseSuite) StartManifold(_ *gc.C) (worker.Worker, error) {
	w, err := s.Manifold.Start(s.Resources.Context())
	if w != nil {
		s.AddCleanup(func(*gc.C) { _ = worker.Stop(w) })
	}
	return w, err
}

func (s *BaseSuite) fakeOpenState(coreagent.Config) (*state.StatePool, error) {
	s.OpenStateCalled = true
	if s.OpenStateErr != nil {
		return nil, s.OpenStateErr
	}
	// Here's one we prepared earlier...
	return s.StatePool, nil
}

type mockAgent struct {
	coreagent.Agent
}

func (ma *mockAgent) CurrentConfig() coreagent.Config {
	return new(mockConfig)
}

type mockConfig struct {
	coreagent.Config
}

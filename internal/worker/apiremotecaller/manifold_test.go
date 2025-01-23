// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	config ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = ManifoldConfig{
		AgentName:      "agent",
		CentralHubName: "central-hub",
		Clock:          clock.WallClock,
	}
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold().Inputs, jc.DeepEquals, []string{"agent", "central-hub"})
}

func (s *ManifoldSuite) TestAgentMissing(c *gc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context.Background(), getter)
	c.Assert(err, jc.ErrorIs, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestCentralHubMissing(c *gc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent":       &fakeAgent{},
		"central-hub": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context.Background(), getter)
	c.Assert(err, jc.ErrorIs, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestAgentAPIInfoNotReady(c *gc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent":       &fakeAgent{missingAPIinfo: true},
		"central-hub": pubsub.NewStructuredHub(nil),
	})

	worker, err := s.manifold().Start(context.Background(), getter)
	c.Assert(err, jc.ErrorIs, dependency.ErrMissing)
	c.Check(worker, gc.IsNil)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *gc.C) {
	clock := s.config.Clock
	hub := pubsub.NewStructuredHub(nil)
	var config WorkerConfig
	s.config.NewWorker = func(c WorkerConfig) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	getter := dt.StubGetter(map[string]interface{}{
		"agent":       &fakeAgent{tag: names.NewMachineTag("42")},
		"central-hub": hub,
	})

	worker, err := s.manifold().Start(context.Background(), getter)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)

	c.Check(config.Origin, gc.Equals, names.NewMachineTag("42"))
	c.Check(config.Clock, gc.Equals, clock)
	c.Check(config.Hub, gc.Equals, hub)
	c.Check(config.APIInfo.CACert, gc.Equals, "fake as")
	c.Check(config.NewRemote, gc.NotNil)
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return Manifold(s.config)
}

type fakeWorker struct {
	worker.Worker
}

type fakeAgent struct {
	agent.Agent

	tag            names.Tag
	missingAPIinfo bool
}

type fakeConfig struct {
	agent.Config

	tag            names.Tag
	missingAPIinfo bool
}

func (f *fakeAgent) CurrentConfig() agent.Config {
	return &fakeConfig{tag: f.tag, missingAPIinfo: f.missingAPIinfo}
}

func (f *fakeConfig) APIInfo() (*api.Info, bool) {
	if f.missingAPIinfo {
		return nil, false
	}
	return &api.Info{
		CACert: "fake as",
		Tag:    f.tag,
	}, true
}

func (f *fakeConfig) Tag() names.Tag {
	return f.tag
}

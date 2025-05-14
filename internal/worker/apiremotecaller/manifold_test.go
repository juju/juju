// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/internal/testhelpers"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	config ManifoldConfig
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = ManifoldConfig{
		AgentName:      "agent",
		CentralHubName: "central-hub",
		Clock:          clock.WallClock,
	}
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"agent", "central-hub"})
}

func (s *ManifoldSuite) TestAgentMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIs, dependency.ErrMissing)
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestCentralHubMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent":       &fakeAgent{},
		"central-hub": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIs, dependency.ErrMissing)
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestAgentAPIInfoNotReady(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent":       &fakeAgent{missingAPIinfo: true},
		"central-hub": pubsub.NewStructuredHub(nil),
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIs, dependency.ErrMissing)
	c.Check(worker, tc.IsNil)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *tc.C) {
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

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(worker, tc.NotNil)

	c.Check(config.Origin, tc.Equals, names.NewMachineTag("42"))
	c.Check(config.Clock, tc.Equals, clock)
	c.Check(config.Hub, tc.Equals, hub)
	c.Check(config.APIInfo.CACert, tc.Equals, "fake as")
	c.Check(config.NewRemote, tc.NotNil)
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

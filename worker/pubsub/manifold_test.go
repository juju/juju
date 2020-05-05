// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	psworker "github.com/juju/juju/worker/pubsub"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config psworker.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = psworker.ManifoldConfig{
		AgentName:      "agent",
		CentralHubName: "central-hub",
		Clock:          testclock.NewClock(time.Now()),
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return psworker.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold().Inputs, jc.DeepEquals, []string{"agent", "central-hub"})
}

func (s *ManifoldSuite) TestAgentMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestCentralHubMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":       &fakeAgent{},
		"central-hub": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestAgentAPIInfoNotReady(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":       &fakeAgent{missingAPIinfo: true},
		"central-hub": pubsub.NewStructuredHub(nil),
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *gc.C) {
	clock := s.config.Clock
	hub := pubsub.NewStructuredHub(nil)
	var config psworker.WorkerConfig
	s.config.NewWorker = func(c psworker.WorkerConfig) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	context := dt.StubContext(nil, map[string]interface{}{
		"agent":       &fakeAgent{tag: names.NewMachineTag("42")},
		"central-hub": hub,
	})

	worker, err := s.manifold().Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)

	c.Check(config.Origin, gc.Equals, "machine-42")
	c.Check(config.Clock, gc.Equals, clock)
	c.Check(config.Hub, gc.Equals, hub)
	c.Check(config.APIInfo.CACert, gc.Equals, "fake as")
	c.Check(config.NewWriter, gc.NotNil)
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

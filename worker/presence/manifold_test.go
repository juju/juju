// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/pubsub"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	corepresence "github.com/juju/juju/core/presence"
	"github.com/juju/juju/worker/presence"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config presence.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = presence.ManifoldConfig{
		AgentName:              "agent",
		CentralHubName:         "central-hub",
		StateConfigWatcherName: "state-config",
		Recorder:               corepresence.New(testclock.NewClock(time.Now())),
		Logger:                 loggo.GetLogger("test"),
		NewWorker: func(presence.WorkerConfig) (worker.Worker, error) {
			return nil, errors.New("boom")
		},
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return presence.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold().Inputs, jc.DeepEquals, []string{"agent", "central-hub", "state-config"})
}

func (s *ManifoldSuite) TestConfigValidation(c *gc.C) {
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestConfigValidationMissingAgentName(c *gc.C) {
	s.config.AgentName = ""
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing AgentName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingCentralHubName(c *gc.C) {
	s.config.CentralHubName = ""
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing CentralHubName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingStateConfigWatcherName(c *gc.C) {
	s.config.StateConfigWatcherName = ""
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing StateConfigWatcherName not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingRecorder(c *gc.C) {
	s.config.Recorder = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing Recorder not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing Logger not valid")
}

func (s *ManifoldSuite) TestConfigValidationMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	err := s.config.Validate()
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "missing NewWorker not valid")
}

func (s *ManifoldSuite) TestConfigNewWorker(c *gc.C) {
	// This test will fail at compile time if the presence.NewWorker function
	// has a different signature to the NewWorker config attribute for ManifoldConfig.
	s.config.NewWorker = presence.NewWorker
}

func (s *ManifoldSuite) TestManifoldCallsValidate(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{})
	s.config.Recorder = nil
	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing Recorder not valid`)
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
		"agent":       &fakeAgent{tag: names.NewMachineTag("42")},
		"central-hub": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNotAServer(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent":        &fakeAgent{tag: names.NewMachineTag("42")},
		"central-hub":  pubsub.NewStructuredHub(nil),
		"state-config": false,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNewWorkerArgs(c *gc.C) {
	hub := pubsub.NewStructuredHub(nil)
	var config presence.WorkerConfig
	s.config.NewWorker = func(c presence.WorkerConfig) (worker.Worker, error) {
		config = c
		return &fakeWorker{}, nil
	}

	context := dt.StubContext(nil, map[string]interface{}{
		"agent":        &fakeAgent{tag: names.NewMachineTag("42")},
		"central-hub":  hub,
		"state-config": true,
	})

	worker, err := s.manifold().Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.NotNil)

	c.Check(config.Origin, gc.Equals, "machine-42")
	c.Check(config.Hub, gc.Equals, hub)
	c.Check(config.Recorder, gc.Equals, s.config.Recorder)
}

type fakeWorker struct {
	worker.Worker
}

type fakeAgent struct {
	agent.Agent
	agent.Config

	tag names.Tag
}

// The fake is its own config.
func (f *fakeAgent) CurrentConfig() agent.Config {
	return f
}

func (f *fakeAgent) Tag() names.Tag {
	return f.tag
}

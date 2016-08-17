// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection_test

import (
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/introspection"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	manifold dependency.Manifold
	startErr error
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.startErr = nil
	s.manifold = introspection.Manifold(introspection.ManifoldConfig{
		AgentName: "agent-name",
		WorkerFunc: func(cfg introspection.Config) (worker.Worker, error) {
			if s.startErr != nil {
				return nil, s.startErr
			}
			return &dummyWorker{config: cfg}, nil
		},
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"agent-name"})
}

func (s *ManifoldSuite) TestStartNonLinux(c *gc.C) {
	if runtime.GOOS == "linux" {
		c.Skip("testing for non-linux")
	}

	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrUninstall)
}

func (s *ManifoldSuite) TestStartAgentMissing(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}

	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartError(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}

	s.startErr = errors.New("boom")
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name": &dummyAgent{},
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("introspection worker not supported on non-linux")
	}

	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name": &dummyAgent{},
	})

	worker, err := s.manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	dummy, ok := worker.(*dummyWorker)
	c.Assert(ok, jc.IsTrue)
	c.Assert(dummy.config.SocketName, gc.Equals, "jujud-machine-42")
}

type dummyAgent struct {
	agent.Agent
}

func (*dummyAgent) CurrentConfig() agent.Config {
	return &dummyConfig{}
}

type dummyConfig struct {
	agent.Config
}

func (*dummyConfig) Tag() names.Tag {
	return names.NewMachineTag("42")
}

type dummyApiCaller struct {
	base.APICaller
}

type dummyWorker struct {
	worker.Worker

	config introspection.Config
}

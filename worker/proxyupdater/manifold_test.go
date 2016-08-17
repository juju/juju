// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/proxyupdater"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config   proxyupdater.ManifoldConfig
	startErr error
}

var _ = gc.Suite(&ManifoldSuite{})

func OtherUpdate(proxy.Settings) error {
	return nil
}

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.startErr = nil
	s.config = proxyupdater.ManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "api-caller-name",
		WorkerFunc: func(cfg proxyupdater.Config) (worker.Worker, error) {
			if s.startErr != nil {
				return nil, s.startErr
			}
			return &dummyWorker{config: cfg}, nil
		},
		ExternalUpdate: OtherUpdate,
	}
}

func (s *ManifoldSuite) manifold() dependency.Manifold {
	return proxyupdater.Manifold(s.config)
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold().Inputs, jc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *ManifoldSuite) TestWorkerFuncMissing(c *gc.C) {
	s.config.WorkerFunc = nil
	context := dt.StubContext(nil, nil)
	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "missing WorkerFunc not valid")
}

func (s *ManifoldSuite) TestStartAgentMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartAPICallerMissing(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartError(c *gc.C) {
	s.startErr = errors.New("boom")
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": &dummyApiCaller{},
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": &dummyApiCaller{},
	})

	worker, err := s.manifold().Start(context)
	c.Check(err, jc.ErrorIsNil)
	dummy, ok := worker.(*dummyWorker)
	c.Assert(ok, jc.IsTrue)
	c.Check(dummy.config.Directory, gc.Equals, "/home/ubuntu")
	c.Check(dummy.config.RegistryPath, gc.Equals, `HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	c.Check(dummy.config.Filename, gc.Equals, ".juju-proxy")
	c.Check(dummy.config.API, gc.NotNil)
	// Checking function equality is problematic.
	c.Check(dummy.config.ExternalUpdate, gc.NotNil)
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

func (*dummyApiCaller) BestFacadeVersion(_ string) int {
	return 42
}

type dummyWorker struct {
	worker.Worker

	config proxyupdater.Config
}

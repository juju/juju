// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/proxy"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/proxyupdater"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config   proxyupdater.ManifoldConfig
	startErr error
}

var _ = gc.Suite(&ManifoldSuite{})

func MakeUpdateFunc(name string) func(proxy.Settings) error {
	// So we can tell the difference between update funcs.
	return func(proxy.Settings) error {
		return errors.New(name)
	}
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
		SupportLegacyValues: true,
		ExternalUpdate:      MakeUpdateFunc("external"),
		InProcessUpdate:     MakeUpdateFunc("in-process"),
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

func (s *ManifoldSuite) TestInProcessUpdateMissing(c *gc.C) {
	s.config.InProcessUpdate = nil
	context := dt.StubContext(nil, nil)
	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "missing InProcessUpdate not valid")
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
		"api-caller-name": &dummyAPICaller{},
	})

	worker, err := s.manifold().Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": &dummyAPICaller{},
	})

	worker, err := s.manifold().Start(context)
	c.Check(err, jc.ErrorIsNil)
	dummy, ok := worker.(*dummyWorker)
	c.Assert(ok, jc.IsTrue)
	c.Check(dummy.config.SystemdFiles, gc.DeepEquals, []string{"/etc/juju-proxy-systemd.conf"})
	c.Check(dummy.config.EnvFiles, gc.DeepEquals, []string{"/etc/juju-proxy.conf"})
	c.Check(dummy.config.RegistryPath, gc.Equals, `HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	c.Check(dummy.config.SupportLegacyValues, jc.IsTrue)
	c.Check(dummy.config.API, gc.NotNil)
	// Checking function equality is problematic, use the errors they
	// return.
	c.Check(dummy.config.ExternalUpdate(proxy.Settings{}), gc.ErrorMatches, "external")
	c.Check(dummy.config.InProcessUpdate(proxy.Settings{}), gc.ErrorMatches, "in-process")
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

type dummyAPICaller struct {
	base.APICaller
}

func (*dummyAPICaller) BestFacadeVersion(_ string) int {
	return 42
}

type dummyWorker struct {
	worker.Worker

	config proxyupdater.Config
}

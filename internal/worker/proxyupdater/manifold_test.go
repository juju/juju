// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/proxy"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/proxyupdater"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
	config   proxyupdater.ManifoldConfig
	startErr error
}

func TestManifoldSuite(t *stdtesting.T) { tc.Run(t, &ManifoldSuite{}) }
func MakeUpdateFunc(name string) func(proxy.Settings) error {
	// So we can tell the difference between update funcs.
	return func(proxy.Settings) error {
		return errors.New(name)
	}
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
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

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *ManifoldSuite) TestWorkerFuncMissing(c *tc.C) {
	s.config.WorkerFunc = nil
	getter := dt.StubGetter(nil)
	worker, err := s.manifold().Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "missing WorkerFunc not valid")
}

func (s *ManifoldSuite) TestInProcessUpdateMissing(c *tc.C) {
	s.config.InProcessUpdate = nil
	getter := dt.StubGetter(nil)
	worker, err := s.manifold().Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "missing InProcessUpdate not valid")
}

func (s *ManifoldSuite) TestStartAgentMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartAPICallerMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartError(c *tc.C) {
	s.startErr = errors.New("boom")
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": &dummyAPICaller{},
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *ManifoldSuite) TestStartSuccess(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"agent-name":      &dummyAgent{},
		"api-caller-name": &dummyAPICaller{},
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Check(err, tc.ErrorIsNil)
	dummy, ok := worker.(*dummyWorker)
	c.Assert(ok, tc.IsTrue)
	c.Check(dummy.config.SystemdFiles, tc.DeepEquals, []string{"/etc/juju-proxy-systemd.conf"})
	c.Check(dummy.config.EnvFiles, tc.DeepEquals, []string{"/etc/juju-proxy.conf"})
	c.Check(dummy.config.SupportLegacyValues, tc.IsTrue)
	c.Check(dummy.config.API, tc.NotNil)
	// Checking function equality is problematic, use the errors they
	// return.
	c.Check(dummy.config.ExternalUpdate(proxy.Settings{}), tc.ErrorMatches, "external")
	c.Check(dummy.config.InProcessUpdate(proxy.Settings{}), tc.ErrorMatches, "in-process")
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

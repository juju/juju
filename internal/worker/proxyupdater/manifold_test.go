// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/proxy"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/testhelpers"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite
	config   ManifoldConfig
	startErr error
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func makeUpdateFunc(name string) func(proxy.Settings) error {
	// So we can tell the difference between update funcs.
	return func(proxy.Settings) error {
		return errors.New(name)
	}
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.startErr = nil
	s.config = ManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "api-caller-name",
		WorkerFunc: func(cfg Config) (worker.Worker, error) {
			if s.startErr != nil {
				return nil, s.startErr
			}
			return &dummyWorker{config: cfg}, nil
		},
		SupportLegacyValues: true,
		ExternalUpdate:      makeUpdateFunc("external"),
		InProcessUpdate:     makeUpdateFunc("in-process"),
		Logger:              logger.GetLogger("test"),
	}
}

func (s *manifoldSuite) manifold() dependency.Manifold {
	return Manifold(s.config)
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"agent-name", "api-caller-name"})
}

func (s *manifoldSuite) TestValidate(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)

	s.config.AgentName = ""
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.APICallerName = ""
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.WorkerFunc = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.ExternalUpdate = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.InProcessUpdate = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.Logger = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStartAgentMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"agent-name": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *manifoldSuite) TestStartAPICallerMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"agent-name":      &dummyAgent{},
		"api-caller-name": dependency.ErrMissing,
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(errors.Cause(err), tc.Equals, dependency.ErrMissing)
}

func (s *manifoldSuite) TestStartError(c *tc.C) {
	s.startErr = errors.New("boom")
	getter := dt.StubGetter(map[string]any{
		"agent-name":      &dummyAgent{},
		"api-caller-name": &dummyAPICaller{},
	})

	worker, err := s.manifold().Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boom")
}

func (s *manifoldSuite) TestStartSuccess(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
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

	config Config
}

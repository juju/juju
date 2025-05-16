// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/remoterelations"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/firewaller"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *stdtesting.T) { tc.Run(t, &ManifoldSuite{}) }
func (s *ManifoldSuite) TestManifoldFirewallModeNone(c *tc.C) {
	ctx := &mockDependencyGetter{
		env: &mockEnviron{
			config: coretesting.CustomModelConfig(c, coretesting.Attrs{
				"firewall-mode": config.FwNone,
			}),
		},
	}

	manifold := firewaller.Manifold(validConfig(c))
	_, err := manifold.Start(c.Context(), ctx)
	c.Assert(err, tc.Equals, dependency.ErrUninstall)
}

type mockDependencyGetter struct {
	dependency.Getter
	env *mockEnviron
}

func (m *mockDependencyGetter) Get(name string, out interface{}) error {
	if name == "environ" {
		*(out.(*environs.Environ)) = m.env
	}
	return nil
}

type mockEnviron struct {
	environs.Environ
	config *config.Config
}

func (e *mockEnviron) Config() *config.Config {
	return e.config
}

type ManifoldConfigSuite struct {
	testhelpers.IsolationSuite
	config firewaller.ManifoldConfig
}

func TestManifoldConfigSuite(t *stdtesting.T) { tc.Run(t, &ManifoldConfigSuite{}) }
func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = validConfig(c)
}

func validConfig(c *tc.C) firewaller.ManifoldConfig {
	return firewaller.ManifoldConfig{
		AgentName:                "agent",
		APICallerName:            "api-caller",
		EnvironName:              "environ",
		Logger:                   loggertesting.WrapCheckLog(c),
		NewControllerConnection:  func(context.Context, *api.Info) (api.Connection, error) { return nil, nil },
		NewFirewallerFacade:      func(base.APICaller) (firewaller.FirewallerAPI, error) { return nil, nil },
		NewFirewallerWorker:      func(firewaller.Config) (worker.Worker, error) { return nil, nil },
		NewRemoteRelationsFacade: func(base.APICaller) *remoterelations.Client { return nil },
	}
}

func (s *ManifoldConfigSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingAgentName(c *tc.C) {
	s.config.AgentName = ""
	s.checkNotValid(c, "empty AgentName not valid")
}

func (s *ManifoldConfigSuite) TestMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingEnvironName(c *tc.C) {
	s.config.EnvironName = ""
	s.checkNotValid(c, "empty EnvironName not valid")
}

func (s *ManifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewFirewallerFacade(c *tc.C) {
	s.config.NewFirewallerFacade = nil
	s.checkNotValid(c, "nil NewFirewallerFacade not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewFirewallerWorker(c *tc.C) {
	s.config.NewFirewallerWorker = nil
	s.checkNotValid(c, "nil NewFirewallerWorker not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewControllerConnection(c *tc.C) {
	s.config.NewControllerConnection = nil
	s.checkNotValid(c, "nil NewControllerConnection not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewRemoteRelationsFacade(c *tc.C) {
	s.config.NewRemoteRelationsFacade = nil
	s.checkNotValid(c, "nil NewRemoteRelationsFacade not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

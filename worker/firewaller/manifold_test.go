// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/firewaller"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestManifoldFirewallModeNone(c *gc.C) {
	ctx := &mockDependencyContext{
		env: &mockEnviron{
			config: coretesting.CustomModelConfig(c, coretesting.Attrs{
				"firewall-mode": config.FwNone,
			}),
		},
	}

	manifold := firewaller.Manifold(firewaller.ManifoldConfig{
		AgentName:                "agent",
		APICallerName:            "api-caller",
		EnvironName:              "environ",
		NewAPIConnForModel:       func(*api.Info) (func(string) (api.Connection, error), error) { return nil, nil },
		NewFirewallerFacade:      func(base.APICaller) (firewaller.FirewallerAPI, error) { return nil, nil },
		NewFirewallerWorker:      func(firewaller.Config) (worker.Worker, error) { return nil, nil },
		NewRemoteRelationsFacade: func(base.APICaller) (*remoterelations.Client, error) { return nil, nil },
	})
	_, err := manifold.Start(ctx)
	c.Assert(err, gc.Equals, dependency.ErrUninstall)
}

type mockDependencyContext struct {
	dependency.Context
	env *mockEnviron
}

func (m *mockDependencyContext) Get(name string, out interface{}) error {
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
	testing.IsolationSuite
	config firewaller.ManifoldConfig
}

var _ = gc.Suite(&ManifoldConfigSuite{})

func (s *ManifoldConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldConfigSuite) validConfig() firewaller.ManifoldConfig {
	return firewaller.ManifoldConfig{
		AgentName:                "agent",
		APICallerName:            "api-caller",
		EnvironName:              "environ",
		NewAPIConnForModel:       func(*api.Info) (func(string) (api.Connection, error), error) { return nil, nil },
		NewFirewallerFacade:      func(base.APICaller) (firewaller.FirewallerAPI, error) { return nil, nil },
		NewFirewallerWorker:      func(firewaller.Config) (worker.Worker, error) { return nil, nil },
		NewRemoteRelationsFacade: func(base.APICaller) (*remoterelations.Client, error) { return nil, nil },
	}
}

func (s *ManifoldConfigSuite) TestValid(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingAgentName(c *gc.C) {
	s.config.AgentName = ""
	s.checkNotValid(c, "empty AgentName not valid")
}

func (s *ManifoldConfigSuite) TestMissingAPICallerName(c *gc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingEnvironName(c *gc.C) {
	s.config.EnvironName = ""
	s.checkNotValid(c, "empty EnvironName not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewFirewallerFacade(c *gc.C) {
	s.config.NewFirewallerFacade = nil
	s.checkNotValid(c, "nil NewFirewallerFacade not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewFirewallerWorker(c *gc.C) {
	s.config.NewFirewallerWorker = nil
	s.checkNotValid(c, "nil NewFirewallerWorker not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewAPIConnForModel(c *gc.C) {
	s.config.NewAPIConnForModel = nil
	s.checkNotValid(c, "nil NewAPIConnForModel not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewRemoteRelationsFacade(c *gc.C) {
	s.config.NewRemoteRelationsFacade = nil
	s.checkNotValid(c, "nil NewRemoteRelationsFacade not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

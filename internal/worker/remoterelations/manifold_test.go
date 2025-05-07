// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/remoterelations"
)

type ManifoldConfigSuite struct {
	testing.IsolationSuite
	config remoterelations.ManifoldConfig
}

var _ = tc.Suite(&ManifoldConfigSuite{})

func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

func (s *ManifoldConfigSuite) validConfig(c *tc.C) remoterelations.ManifoldConfig {
	return remoterelations.ManifoldConfig{
		AgentName:                "agent",
		APICallerName:            "api-caller",
		NewControllerConnection:  func(context.Context, *api.Info) (api.Connection, error) { return nil, nil },
		NewRemoteRelationsFacade: func(base.APICaller) remoterelations.RemoteRelationsFacade { return nil },
		NewWorker:                func(remoterelations.Config) (worker.Worker, error) { return nil, nil },
		Logger:                   loggertesting.WrapCheckLog(c),
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

func (s *ManifoldConfigSuite) TestMissingNewRemoteRelationsFacade(c *tc.C) {
	s.config.NewRemoteRelationsFacade = nil
	s.checkNotValid(c, "nil NewRemoteRelationsFacade not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewControllerConnection(c *tc.C) {
	s.config.NewControllerConnection = nil
	s.checkNotValid(c, "nil NewControllerConnection not valid")
}

func (s *ManifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

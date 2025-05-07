// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/migrationminion"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config migrationminion.ManifoldConfig
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

func (s *ManifoldSuite) validConfig(c *tc.C) migrationminion.ManifoldConfig {
	return migrationminion.ManifoldConfig{
		AgentName:         "agent",
		APICallerName:     "api-caller",
		FortressName:      "fortress",
		Clock:             struct{ clock.Clock }{},
		APIOpen:           func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) { return nil, nil },
		ValidateMigration: func(context.Context, base.APICaller) error { return nil },
		NewFacade:         func(base.APICaller) (migrationminion.Facade, error) { return nil, nil },
		NewWorker:         func(migrationminion.Config) (worker.Worker, error) { return nil, nil },
		Logger:            loggertesting.WrapCheckLog(c),
	}
}

func (s *ManifoldSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestMissingAgentName(c *tc.C) {
	s.config.AgentName = ""
	s.checkNotValid(c, "empty AgentName not valid")
}

func (s *ManifoldSuite) TestMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldSuite) TestMissingFortressName(c *tc.C) {
	s.config.FortressName = ""
	s.checkNotValid(c, "empty FortressName not valid")
}

func (s *ManifoldSuite) TestMissingClock(c *tc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *ManifoldSuite) TestMissingAPIOpen(c *tc.C) {
	s.config.APIOpen = nil
	s.checkNotValid(c, "nil APIOpen not valid")
}

func (s *ManifoldSuite) TestMissingValidateMigration(c *tc.C) {
	s.config.ValidateMigration = nil
	s.checkNotValid(c, "nil ValidateMigration not valid")
}

func (s *ManifoldSuite) TestMissingNewFacade(c *tc.C) {
	s.config.NewFacade = nil
	s.checkNotValid(c, "nil NewFacade not valid")
}

func (s *ManifoldSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion_test

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/worker/migrationminion"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	config migrationminion.ManifoldConfig
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldSuite) validConfig() migrationminion.ManifoldConfig {
	return migrationminion.ManifoldConfig{
		AgentName:         "agent",
		APICallerName:     "api-caller",
		FortressName:      "fortress",
		Clock:             struct{ clock.Clock }{},
		APIOpen:           func(*api.Info, api.DialOpts) (api.Connection, error) { return nil, nil },
		ValidateMigration: func(base.APICaller) error { return nil },
		NewFacade:         func(base.APICaller) (migrationminion.Facade, error) { return nil, nil },
		NewWorker:         func(migrationminion.Config) (worker.Worker, error) { return nil, nil },
		Logger:            loggo.GetLogger("test"),
	}
}

func (s *ManifoldSuite) TestValid(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldSuite) TestMissingAgentName(c *gc.C) {
	s.config.AgentName = ""
	s.checkNotValid(c, "empty AgentName not valid")
}

func (s *ManifoldSuite) TestMissingAPICallerName(c *gc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldSuite) TestMissingFortressName(c *gc.C) {
	s.config.FortressName = ""
	s.checkNotValid(c, "empty FortressName not valid")
}

func (s *ManifoldSuite) TestMissingClock(c *gc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *ManifoldSuite) TestMissingAPIOpen(c *gc.C) {
	s.config.APIOpen = nil
	s.checkNotValid(c, "nil APIOpen not valid")
}

func (s *ManifoldSuite) TestMissingValidateMigration(c *gc.C) {
	s.config.ValidateMigration = nil
	s.checkNotValid(c, "nil ValidateMigration not valid")
}

func (s *ManifoldSuite) TestMissingNewFacade(c *gc.C) {
	s.config.NewFacade = nil
	s.checkNotValid(c, "nil NewFacade not valid")
}

func (s *ManifoldSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

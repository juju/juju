// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pruner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker/pruner"
)

type ManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ManifoldSuite{})

type ManifoldConfigSuite struct {
	testing.IsolationSuite
	config pruner.ManifoldConfig
}

var _ = gc.Suite(&ManifoldConfigSuite{})

func (s *ManifoldConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldConfigSuite) validConfig() pruner.ManifoldConfig {
	return pruner.ManifoldConfig{
		APICallerName: "api-caller",
		EnvironName:   "environ",
		ClockName:     "clock",
		NewWorker:     func(pruner.Config) (worker.Worker, error) { return nil, nil },
		NewFacade:     func(caller base.APICaller) pruner.Facade { return nil },
	}
}

func (s *ManifoldConfigSuite) TestValid(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingAPICallerName(c *gc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingEnvironName(c *gc.C) {
	s.config.EnvironName = ""
	s.checkNotValid(c, "empty EnvironName not valid")
}

func (s *ManifoldConfigSuite) TestMissingClockName(c *gc.C) {
	s.config.ClockName = ""
	s.checkNotValid(c, "empty ClockName not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewFacade(c *gc.C) {
	s.config.NewFacade = nil
	s.checkNotValid(c, "nil NewFacade not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

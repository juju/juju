// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/worker/caasoperatorprovisioner"
)

type ManifoldConfigSuite struct {
	testing.IsolationSuite
	config caasoperatorprovisioner.ManifoldConfig
}

var _ = gc.Suite(&ManifoldConfigSuite{})

func (s *ManifoldConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldConfigSuite) validConfig() caasoperatorprovisioner.ManifoldConfig {
	return caasoperatorprovisioner.ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
		BrokerName:    "broker",
		NewWorker: func(config caasoperatorprovisioner.Config) (worker.Worker, error) {
			return nil, nil
		},
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

func (s *ManifoldConfigSuite) TestMissingBrokerName(c *gc.C) {
	s.config.BrokerName = ""
	s.checkNotValid(c, "empty BrokerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

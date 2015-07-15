// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
)

type rsyslogPortSuite struct {
	jujutesting.JujuConnSuite
	ctx upgrades.Context
}

var _ = gc.Suite(&rsyslogPortSuite{})

func (s *rsyslogPortSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	apiState, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.ctx = &mockContext{
		agentConfig: &mockAgentConfig{
			dataDir:    s.DataDir(),
			mongoInfo:  s.MongoInfo(c),
			environTag: s.State.EnvironTag(),
		},
		apiState: apiState,
		state:    s.State,
	}
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.SyslogPort(), gc.Not(gc.Equals), config.DefaultSyslogPort)
}

func (s *rsyslogPortSuite) TestSyslogPortChanged(c *gc.C) {
	err := upgrades.UpdateRsyslogPort(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.SyslogPort(), gc.Equals, config.DefaultSyslogPort)
}

func (s *rsyslogPortSuite) TestIdempotent(c *gc.C) {
	err := upgrades.UpdateRsyslogPort(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	err = upgrades.UpdateRsyslogPort(s.ctx)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := s.State.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.SyslogPort(), gc.Equals, config.DefaultSyslogPort)
}

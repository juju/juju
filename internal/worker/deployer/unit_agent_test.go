// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/core/logger"
	jv "github.com/juju/juju/core/version"
	internaldependency "github.com/juju/juju/internal/dependency"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jt "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/internal/worker/deployer"
)

type UnitAgentSuite struct {
	BaseSuite

	workers *unitWorkersStub
	config  deployer.UnitAgentConfig
}

var _ = gc.Suite(&UnitAgentSuite{})

func (s *UnitAgentSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.workers = &unitWorkersStub{
		started: make(chan string, 10), // eval size later
		stopped: make(chan string, 10), // eval size later
		logger:  loggertesting.WrapCheckLog(c).Child("unit-agent"),
	}

	s.config = deployer.UnitAgentConfig{
		Name:         "someunit/42",
		DataDir:      c.MkDir(),
		Clock:        clock.WallClock,
		Logger:       loggertesting.WrapCheckLog(c).Child("unit-agent"),
		SetupLogging: func(logger.LoggerContext, agent.Config) {},
		UnitEngineConfig: func() dependency.EngineConfig {
			return engine.DependencyEngineConfig(
				dependency.DefaultMetrics(),
				internaldependency.WrapLogger(loggertesting.WrapCheckLog(c).Child("dependency")),
			)
		},
		UnitManifolds: s.workers.Manifolds,
	}
}

func (s *UnitAgentSuite) TestConfigMissingName(c *gc.C) {
	s.config.Name = ""
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Name not valid")
}

func (s *UnitAgentSuite) TestConfigMissingDataDir(c *gc.C) {
	s.config.DataDir = ""
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing DataDir not valid")
}

func (s *UnitAgentSuite) TestConfigMissingClock(c *gc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Clock not valid")
}

func (s *UnitAgentSuite) TestConfigMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing Logger not valid")
}

func (s *UnitAgentSuite) TestConfigMissingSetupLogging(c *gc.C) {
	s.config.SetupLogging = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing SetupLogging not valid")
}

func (s *UnitAgentSuite) TestConfigMissingUnitEngineConfig(c *gc.C) {
	s.config.UnitEngineConfig = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing UnitEngineConfig not valid")
}

func (s *UnitAgentSuite) TestConfigMissingUnitManifolds(c *gc.C) {
	s.config.UnitManifolds = nil
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), gc.Equals, "missing UnitManifolds not valid")
}

func (s *UnitAgentSuite) writeAgentConf(c *gc.C) {
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir:         s.config.DataDir,
				LogDir:          c.MkDir(),
				MetricsSpoolDir: agent.DefaultPaths.MetricsSpoolDir,
			},
			Tag:          names.NewUnitTag(s.config.Name),
			Password:     "sekrit",
			Nonce:        "unused",
			Controller:   jt.ControllerTag,
			Model:        jt.ModelTag,
			APIAddresses: []string{"unused:1234"},
			CACert:       jt.CACert,
			// We'll use an old version number here to confirm
			// that it gets updated.
			UpgradedToVersion: version.Number{Major: 2, Minor: 2},
		})
	c.Assert(err, jc.ErrorIsNil)
	err = conf.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UnitAgentSuite) newUnitAgent(c *gc.C) *deployer.UnitAgent {
	s.InitializeCurrentToolsDir(c, s.config.DataDir)
	agent, err := deployer.NewUnitAgent(s.config)
	c.Assert(err, jc.ErrorIsNil)
	return agent
}

func (s *UnitAgentSuite) TestNewAgentSetsUpgradedToVersion(c *gc.C) {
	s.writeAgentConf(c)
	agent := s.newUnitAgent(c)
	config := agent.CurrentConfig()
	c.Assert(config.UpgradedToVersion(), gc.Equals, jv.Current)
}

func (s *UnitAgentSuite) TestChangeConfigWritesChanges(c *gc.C) {
	s.writeAgentConf(c)
	ua := s.newUnitAgent(c)
	err := ua.ChangeConfig(func(setter agent.ConfigSetter) error {
		setter.SetValue("foo", "bar")
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	ub := s.newUnitAgent(c)
	config := ub.CurrentConfig()
	c.Assert(config.Value("foo"), gc.Equals, "bar")
}

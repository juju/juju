// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	jv "github.com/juju/juju/core/version"
	internaldependency "github.com/juju/juju/internal/dependency"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jt "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/deployer"
)

type UnitAgentSuite struct {
	BaseSuite

	workers *unitWorkersStub
	config  deployer.UnitAgentConfig
}

func TestUnitAgentSuite(t *stdtesting.T) {
	tc.Run(t, &UnitAgentSuite{})
}

func (s *UnitAgentSuite) SetUpTest(c *tc.C) {
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

func (s *UnitAgentSuite) TestConfigMissingName(c *tc.C) {
	s.config.Name = ""
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), tc.Equals, "missing Name not valid")
}

func (s *UnitAgentSuite) TestConfigMissingDataDir(c *tc.C) {
	s.config.DataDir = ""
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), tc.Equals, "missing DataDir not valid")
}

func (s *UnitAgentSuite) TestConfigMissingClock(c *tc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), tc.Equals, "missing Clock not valid")
}

func (s *UnitAgentSuite) TestConfigMissingLogger(c *tc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), tc.Equals, "missing Logger not valid")
}

func (s *UnitAgentSuite) TestConfigMissingSetupLogging(c *tc.C) {
	s.config.SetupLogging = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), tc.Equals, "missing SetupLogging not valid")
}

func (s *UnitAgentSuite) TestConfigMissingUnitEngineConfig(c *tc.C) {
	s.config.UnitEngineConfig = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), tc.Equals, "missing UnitEngineConfig not valid")
}

func (s *UnitAgentSuite) TestConfigMissingUnitManifolds(c *tc.C) {
	s.config.UnitManifolds = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Assert(err.Error(), tc.Equals, "missing UnitManifolds not valid")
}

func (s *UnitAgentSuite) writeAgentConf(c *tc.C) {
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
			UpgradedToVersion: semversion.Number{Major: 2, Minor: 2},
		})
	c.Assert(err, tc.ErrorIsNil)
	err = conf.Write()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *UnitAgentSuite) newUnitAgent(c *tc.C) *deployer.UnitAgent {
	s.InitializeCurrentToolsDir(c, s.config.DataDir)
	agent, err := deployer.NewUnitAgent(s.config)
	c.Assert(err, tc.ErrorIsNil)
	return agent
}

func (s *UnitAgentSuite) TestNewAgentSetsUpgradedToVersion(c *tc.C) {
	s.writeAgentConf(c)
	agent := s.newUnitAgent(c)
	config := agent.CurrentConfig()
	c.Assert(config.UpgradedToVersion(), tc.Equals, jv.Current)
}

func (s *UnitAgentSuite) TestChangeConfigWritesChanges(c *tc.C) {
	s.writeAgentConf(c)
	ua := s.newUnitAgent(c)
	err := ua.ChangeConfig(func(setter agent.ConfigSetter) error {
		setter.SetValue("foo", "bar")
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	ub := s.newUnitAgent(c)
	config := ub.CurrentConfig()
	c.Assert(config.Value("foo"), tc.Equals, "bar")
}

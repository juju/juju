// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/utils/syslog"
	syslogtesting "launchpad.net/juju-core/utils/syslog/testing"
)

type syslogConfigSuite struct {
	testing.IsolationSuite
	configDir string
}

var _ = gc.Suite(&syslogConfigSuite{})

func (s *syslogConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.configDir = c.MkDir()
}

func (s *syslogConfigSuite) assertRsyslogConfigPath(c *gc.C, slConfig *syslog.SyslogConfig) {
	slConfig.ConfigDir = s.configDir
	slConfig.ConfigFileName = "rsyslog.conf"
	c.Assert(slConfig.ConfigFilePath(), gc.Equals, filepath.Join(s.configDir, "rsyslog.conf"))
}

func (s *syslogConfigSuite) assertRsyslogConfigContents(c *gc.C, slConfig *syslog.SyslogConfig,
	expectedConf string) {
	data, err := slConfig.Render()
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, expectedConf)
}

func (s *syslogConfigSuite) TestAccumulateConfigRender(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine", agent.DefaultLogDir, 8888, "", []string{"foo"})
	s.assertRsyslogConfigContents(
		c,
		syslogConfigRenderer,
		syslogtesting.ExpectedAccumulateSyslogConf(c, "some-machine", "", 8888),
	)
}

func (s *syslogConfigSuite) TestAccumulateConfigWrite(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine", agent.DefaultLogDir, 8888, "", []string{"foo"})
	syslogConfigRenderer.ConfigDir = s.configDir
	syslogConfigRenderer.ConfigFileName = "rsyslog.conf"
	s.assertRsyslogConfigPath(c, syslogConfigRenderer)
	err := syslogConfigRenderer.Write()
	c.Assert(err, gc.IsNil)
	syslogConfData, err := ioutil.ReadFile(syslogConfigRenderer.ConfigFilePath())
	c.Assert(err, gc.IsNil)
	c.Assert(
		string(syslogConfData),
		gc.Equals,
		syslogtesting.ExpectedAccumulateSyslogConf(c, "some-machine", "", 8888),
	)
}

func (s *syslogConfigSuite) TestAccumulateConfigRenderWithNamespace(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine", agent.DefaultLogDir, 8888, "namespace", []string{"foo"})
	syslogConfigRenderer.LogDir += "-namespace"
	s.assertRsyslogConfigContents(
		c, syslogConfigRenderer, syslogtesting.ExpectedAccumulateSyslogConf(
			c, "some-machine", "namespace", 8888,
		),
	)
}

func (s *syslogConfigSuite) TestForwardConfigRender(c *gc.C) {
	syslogConfigRenderer := syslog.NewForwardConfig(
		"some-machine", agent.DefaultLogDir, 999, "", []string{"server"},
	)
	s.assertRsyslogConfigContents(
		c, syslogConfigRenderer, syslogtesting.ExpectedForwardSyslogConf(
			c, "some-machine", agent.DefaultLogDir, "", "server", 999,
		),
	)
}

func (s *syslogConfigSuite) TestForwardConfigRenderWithNamespace(c *gc.C) {
	syslogConfigRenderer := syslog.NewForwardConfig(
		"some-machine", agent.DefaultLogDir, 999, "namespace", []string{"server"},
	)
	s.assertRsyslogConfigContents(
		c, syslogConfigRenderer, syslogtesting.ExpectedForwardSyslogConf(
			c, "some-machine", agent.DefaultLogDir, "namespace", "server", 999,
		),
	)
}

func (s *syslogConfigSuite) TestForwardConfigWrite(c *gc.C) {
	syslogConfigRenderer := syslog.NewForwardConfig(
		"some-machine", agent.DefaultLogDir, 999, "", []string{"server"},
	)
	syslogConfigRenderer.ConfigDir = s.configDir
	syslogConfigRenderer.ConfigFileName = "rsyslog.conf"
	s.assertRsyslogConfigPath(c, syslogConfigRenderer)
	err := syslogConfigRenderer.Write()
	c.Assert(err, gc.IsNil)
	syslogConfData, err := ioutil.ReadFile(syslogConfigRenderer.ConfigFilePath())
	c.Assert(err, gc.IsNil)
	c.Assert(
		string(syslogConfData),
		gc.Equals,
		syslogtesting.ExpectedForwardSyslogConf(
			c, "some-machine", agent.DefaultLogDir, "", "server", 999,
		),
	)
}

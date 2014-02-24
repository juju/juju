// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	"io/ioutil"
	"path/filepath"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/log/syslog"
	syslogtesting "launchpad.net/juju-core/log/syslog/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type SyslogConfigSuite struct {
	configDir string
}

var _ = gc.Suite(&SyslogConfigSuite{})

func (s *SyslogConfigSuite) SetUpTest(c *gc.C) {
	s.configDir = c.MkDir()
}

func (s *SyslogConfigSuite) assertRsyslogConfigPath(c *gc.C, slConfig *syslog.SyslogConfig) {
	slConfig.ConfigDir = s.configDir
	slConfig.ConfigFileName = "rsyslog.conf"
	c.Assert(slConfig.ConfigFilePath(), gc.Equals, filepath.Join(s.configDir, "rsyslog.conf"))
}

func (s *SyslogConfigSuite) assertRsyslogConfigContents(c *gc.C, slConfig *syslog.SyslogConfig,
	expectedConf string) {
	data, err := slConfig.Render()
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, expectedConf)
}

func (s *SyslogConfigSuite) TestAccumulateConfigRender(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine", 8888, "")
	s.assertRsyslogConfigContents(
		c, syslogConfigRenderer, syslogtesting.ExpectedAccumulateSyslogConf(c, "some-machine", "", 8888, false))
}

func (s *SyslogConfigSuite) TestAccumulateConfigRenderTLS(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine", 8888, "")
	syslogConfigRenderer.TLSCACertPath = "/var/log/juju/ca.pem"
	syslogConfigRenderer.TLSCertPath = "/var/log/juju/cert.pem"
	syslogConfigRenderer.TLSKeyPath = "/var/log/juju/key.pem"
	s.assertRsyslogConfigContents(
		c, syslogConfigRenderer, syslogtesting.ExpectedAccumulateSyslogConf(c, "some-machine", "", 8888, true))
}

func (s *SyslogConfigSuite) TestAccumulateConfigWrite(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine", 8888, "")
	syslogConfigRenderer.ConfigDir = s.configDir
	syslogConfigRenderer.ConfigFileName = "rsyslog.conf"
	s.assertRsyslogConfigPath(c, syslogConfigRenderer)
	err := syslogConfigRenderer.Write()
	c.Assert(err, gc.IsNil)
	syslogConfData, err := ioutil.ReadFile(syslogConfigRenderer.ConfigFilePath())
	c.Assert(err, gc.IsNil)
	c.Assert(string(syslogConfData), gc.Equals, syslogtesting.ExpectedAccumulateSyslogConf(c, "some-machine", "", 8888, false))
}

func (s *SyslogConfigSuite) TestAccumulateConfigRenderWithNamespace(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine", 8888, "namespace")
	s.assertRsyslogConfigContents(
		c, syslogConfigRenderer, syslogtesting.ExpectedAccumulateSyslogConf(c, "some-machine", "namespace", 8888, false))
}

func (s *SyslogConfigSuite) TestForwardConfigRender(c *gc.C) {
	syslogConfigRenderer := syslog.NewForwardConfig("some-machine", 999, "", []string{"server"})
	s.assertRsyslogConfigContents(
		c, syslogConfigRenderer, syslogtesting.ExpectedForwardSyslogConf(c, "some-machine", "", 999, false))
}

func (s *SyslogConfigSuite) TestForwardConfigRenderTLS(c *gc.C) {
	syslogConfigRenderer := syslog.NewForwardConfig("some-machine", 999, "", []string{"server"})
	syslogConfigRenderer.TLSCACertPath = "/var/log/juju/ca.pem"
	s.assertRsyslogConfigContents(
		c, syslogConfigRenderer, syslogtesting.ExpectedForwardSyslogConf(c, "some-machine", "", 999, true))
}

func (s *SyslogConfigSuite) TestForwardConfigRenderWithNamespace(c *gc.C) {
	syslogConfigRenderer := syslog.NewForwardConfig("some-machine", 999, "namespace", []string{"server"})
	s.assertRsyslogConfigContents(
		c, syslogConfigRenderer, syslogtesting.ExpectedForwardSyslogConf(c, "some-machine", "namespace", 999, false))
}

func (s *SyslogConfigSuite) TestForwardConfigWrite(c *gc.C) {
	syslogConfigRenderer := syslog.NewForwardConfig("some-machine", 999, "", []string{"server"})
	syslogConfigRenderer.ConfigDir = s.configDir
	syslogConfigRenderer.ConfigFileName = "rsyslog.conf"
	s.assertRsyslogConfigPath(c, syslogConfigRenderer)
	err := syslogConfigRenderer.Write()
	c.Assert(err, gc.IsNil)
	syslogConfData, err := ioutil.ReadFile(syslogConfigRenderer.ConfigFilePath())
	c.Assert(err, gc.IsNil)
	c.Assert(string(syslogConfData), gc.Equals, syslogtesting.ExpectedForwardSyslogConf(c, "some-machine", "", 999, false))
}

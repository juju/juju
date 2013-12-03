// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/log/syslog"
)

func Test(t *testing.T) {
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

var expectedAccumulateSyslogConf = `
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/some-machine.log
$InputFileTag juju-some-machine:
$InputFileStateFile some-machine
$InputRunFileMonitor

$ModLoad imudp
$UDPServerRun 8888

# Messages received from remote rsyslog machines have messages prefixed with a space,
# so add one in for local messages too if needed.
$template JujuLogFormat,"%syslogtag:6:$%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n"

:syslogtag, startswith, "juju-" /var/log/juju/all-machines.log;JujuLogFormat
& ~
`

func (s *SyslogConfigSuite) TestAccumulateConfigRender(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine", 8888, "")
	s.assertRsyslogConfigContents(c, syslogConfigRenderer, expectedAccumulateSyslogConf)
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
	c.Assert(string(syslogConfData), gc.Equals, expectedAccumulateSyslogConf)
}

var expectedAccumulateNamespaceSyslogConf = `
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/some-machine.log
$InputFileTag juju-namespace-some-machine:
$InputFileStateFile some-machine-namespace
$InputRunFileMonitor

$ModLoad imudp
$UDPServerRun 8888

# Messages received from remote rsyslog machines have messages prefixed with a space,
# so add one in for local messages too if needed.
$template JujuLogFormat-namespace,"%syslogtag:16:$%%msg:::sp-if-no-1st-sp%%msg:::drop-last-lf%\n"

:syslogtag, startswith, "juju-namespace-" /var/log/juju/all-machines.log;JujuLogFormat-namespace
& ~
`

func (s *SyslogConfigSuite) TestAccumulateConfigRenderWithNamespace(c *gc.C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine", 8888, "namespace")
	s.assertRsyslogConfigContents(c, syslogConfigRenderer, expectedAccumulateNamespaceSyslogConf)
}

var expectedForwardSyslogConf = `
$ModLoad imfile

$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/some-machine.log
$InputFileTag juju-some-machine:
$InputFileStateFile some-machine
$InputRunFileMonitor

$template LongTagForwardFormat,"<%PRI%>%TIMESTAMP:::date-rfc3339% %HOSTNAME% %syslogtag%%msg:::sp-if-no-1st-sp%%msg%"

:syslogtag, startswith, "juju-" @server:999;LongTagForwardFormat
& ~
`

func (s *SyslogConfigSuite) TestForwardConfigRender(c *gc.C) {
	syslogConfigRenderer := syslog.NewForwardConfig("some-machine", 999, "", []string{"server"})
	s.assertRsyslogConfigContents(c, syslogConfigRenderer, expectedForwardSyslogConf)
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
	c.Assert(string(syslogConfData), gc.Equals, expectedForwardSyslogConf)
}

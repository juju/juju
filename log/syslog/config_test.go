// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/log/syslog"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type SyslogConfigSuite struct {
	configDir string
}

var _ = Suite(&SyslogConfigSuite{})

func (s *SyslogConfigSuite) SetUpTest(c *C) {
	s.configDir = c.MkDir()
}

func (s *SyslogConfigSuite) assertRsyslogConfigPath(c *C, slConfig *syslog.SyslogConfig) {
	slConfig.ConfigDir = s.configDir
	slConfig.ConfigFileName = "rsyslog.conf"
	c.Assert(slConfig.ConfigFilePath(), Equals, filepath.Join(s.configDir, "rsyslog.conf"))
}

func (s *SyslogConfigSuite) assertRsyslogConfigContents(c *C, slConfig *syslog.SyslogConfig,
	expectedConf string) {
	data, err := slConfig.Render()
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, expectedConf)
}

var expectedAccumulateSyslogConf = `
$ModLoad imfile

$InputFileStateFile /var/spool/rsyslog/juju-some-machine-state
$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/some-machine.log
$InputFileTag local-juju-some-machine:
$InputFileStateFile some-machine
$InputRunFileMonitor

$ModLoad imudp
$UDPServerRun 514

# Messages received from remote rsyslog machines contain a leading space so we
# need to account for that.
$template JujuLogFormatLocal,"%HOSTNAME%:%msg:::drop-last-lf%\n"
$template JujuLogFormat,"%HOSTNAME%:%msg:2:2048:drop-last-lf%\n"

:syslogtag, startswith, "juju-" /var/log/juju/all-machines.log;JujuLogFormat
:syslogtag, startswith, "local-juju-" /var/log/juju/all-machines.log;JujuLogFormatLocal
& ~
`

func (s *SyslogConfigSuite) TestAccumulateConfigRender(c *C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine")
	s.assertRsyslogConfigContents(c, syslogConfigRenderer, expectedAccumulateSyslogConf)
}

func (s *SyslogConfigSuite) TestAccumulateConfigWrite(c *C) {
	syslogConfigRenderer := syslog.NewAccumulateConfig("some-machine")
	syslogConfigRenderer.ConfigDir = s.configDir
	syslogConfigRenderer.ConfigFileName = "rsyslog.conf"
	s.assertRsyslogConfigPath(c, syslogConfigRenderer)
	err := syslogConfigRenderer.Write()
	c.Assert(err, IsNil)
	syslogConfData, err := ioutil.ReadFile(syslogConfigRenderer.ConfigFilePath())
	c.Assert(err, IsNil)
	c.Assert(string(syslogConfData), Equals, expectedAccumulateSyslogConf)
}

var expectedForwardSyslogConf = `
$ModLoad imfile

$InputFileStateFile /var/spool/rsyslog/juju-some-machine-state
$InputFilePersistStateInterval 50
$InputFilePollInterval 5
$InputFileName /var/log/juju/some-machine.log
$InputFileTag juju-some-machine:
$InputFileStateFile some-machine
$InputRunFileMonitor

:syslogtag, startswith, "juju-" @server:514
& ~
`

func (s *SyslogConfigSuite) TestForwardConfigRender(c *C) {
	syslogConfigRenderer := syslog.NewForwardConfig("some-machine", []string{"server"})
	s.assertRsyslogConfigContents(c, syslogConfigRenderer, expectedForwardSyslogConf)
}

func (s *SyslogConfigSuite) TestForwardConfigWrite(c *C) {
	syslogConfigRenderer := syslog.NewForwardConfig("some-machine", []string{"server"})
	syslogConfigRenderer.ConfigDir = s.configDir
	syslogConfigRenderer.ConfigFileName = "rsyslog.conf"
	s.assertRsyslogConfigPath(c, syslogConfigRenderer)
	err := syslogConfigRenderer.Write()
	c.Assert(err, IsNil)
	syslogConfData, err := ioutil.ReadFile(syslogConfigRenderer.ConfigFilePath())
	c.Assert(err, IsNil)
	c.Assert(string(syslogConfData), Equals, expectedForwardSyslogConf)
}

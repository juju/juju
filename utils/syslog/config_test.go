// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/utils/syslog"
	syslogtesting "github.com/juju/juju/utils/syslog/testing"
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

func (s *syslogConfigSuite) assertRsyslogConfigContents(c *gc.C, slConfig *syslog.SyslogConfig, expectedConf string) {
	data, err := slConfig.Render()
	c.Assert(err, jc.ErrorIsNil)
	if len(data) == 0 {
		c.Fatal("got empty data from render")
	}
	d := string(data)
	if d != expectedConf {
		diff(c, d, expectedConf)
		c.Fail()
	}
}

func args() syslogtesting.TemplateArgs {
	return syslogtesting.TemplateArgs{
		MachineTag: "some-machine",
		LogDir:     agent.DefaultLogDir,
		DataDir:    agent.DefaultDataDir,
		Port:       8888,
		Server:     "server",
	}
}

func cfg() *syslog.SyslogConfig {
	return &syslog.SyslogConfig{
		LogFileName:          "some-machine",
		LogDir:               agent.DefaultLogDir,
		JujuConfigDir:        agent.DefaultDataDir,
		Port:                 8888,
		StateServerAddresses: []string{"server"},
	}
}

func (s *syslogConfigSuite) TestAccumulateConfigRender(c *gc.C) {
	cfg := cfg()
	syslog.NewAccumulateConfig(cfg)
	s.assertRsyslogConfigContents(
		c,
		cfg,
		syslogtesting.ExpectedAccumulateSyslogConf(c, args()),
	)
}

func (s *syslogConfigSuite) TestAccumulateConfigWrite(c *gc.C) {
	syslogConfigRenderer := cfg()
	syslog.NewAccumulateConfig(syslogConfigRenderer)
	syslogConfigRenderer.ConfigDir = s.configDir
	syslogConfigRenderer.ConfigFileName = "rsyslog.conf"
	s.assertRsyslogConfigPath(c, syslogConfigRenderer)
	err := syslogConfigRenderer.Write()
	c.Assert(err, jc.ErrorIsNil)
	syslogConfData, err := ioutil.ReadFile(syslogConfigRenderer.ConfigFilePath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		string(syslogConfData),
		gc.Equals,
		syslogtesting.ExpectedAccumulateSyslogConf(c, args()),
	)
}

func (s *syslogConfigSuite) TestAccumulateConfigRenderWithNamespace(c *gc.C) {
	cfg := cfg()
	cfg.Namespace = "namespace"
	cfg.JujuConfigDir = cfg.JujuConfigDir + "-" + cfg.Namespace
	cfg.LogDir = cfg.LogDir + "-" + cfg.Namespace

	args := args()
	args.Namespace = "namespace"
	syslog.NewAccumulateConfig(cfg)
	s.assertRsyslogConfigContents(
		c,
		cfg,
		syslogtesting.ExpectedAccumulateSyslogConf(c, args),
	)
}

func (s *syslogConfigSuite) TestForwardConfigRender(c *gc.C) {
	cfg := cfg()
	syslog.NewForwardConfig(cfg)
	s.assertRsyslogConfigContents(
		c,
		cfg,
		syslogtesting.ExpectedForwardSyslogConf(c, args()),
	)
}

func (s *syslogConfigSuite) TestForwardConfigRenderWithNamespace(c *gc.C) {
	cfg := cfg()
	cfg.Namespace = "namespace"
	args := args()
	args.Namespace = "namespace"
	syslog.NewForwardConfig(cfg)
	s.assertRsyslogConfigContents(
		c,
		cfg,
		syslogtesting.ExpectedForwardSyslogConf(c, args),
	)
}

func (s *syslogConfigSuite) TestForwardConfigWrite(c *gc.C) {
	syslogConfigRenderer := cfg()
	syslogConfigRenderer.ConfigDir = s.configDir
	syslogConfigRenderer.ConfigFileName = "rsyslog.conf"
	syslog.NewForwardConfig(syslogConfigRenderer)
	s.assertRsyslogConfigPath(c, syslogConfigRenderer)
	err := syslogConfigRenderer.Write()
	c.Assert(err, jc.ErrorIsNil)
	syslogConfData, err := ioutil.ReadFile(syslogConfigRenderer.ConfigFilePath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(
		string(syslogConfData),
		gc.Equals,
		syslogtesting.ExpectedForwardSyslogConf(c, args()),
	)
}

func diff(c *gc.C, got, exp string) {
	expR := []rune(exp)
	gotR := []rune(got)
	for x := 0; x < len(expR); x++ {
		if x >= len(gotR) {
			c.Log("String obtained is truncated version of expected.")
			c.Errorf("Expected: %s, got: %s", exp, got)
			return
		}
		if expR[x] != gotR[x] {
			c.Logf("Diff at offset %d", x)
			gotDiff := string(gotR[x:min(x+50, len(gotR)-x)])
			expDiff := string(expR[x:min(x+50, len(expR)-x)])
			c.Logf("Diff at offset - obtained: %#v\nexpected: %#v", gotDiff, expDiff)
			c.Assert(got, gc.Equals, exp)
			return
		}
	}
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

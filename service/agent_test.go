// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"path/filepath"
	"runtime"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/shell"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

var (
	cmdSuffix string
	shquote   = utils.ShQuote
)

func init() {
	if runtime.GOOS == "windows" {
		cmdSuffix = ".exe"
	}
}

type agentSuite struct {
	service.BaseSuite
}

var _ = gc.Suite(&agentSuite{})

func (*agentSuite) TestAgentConfMachineLocal(c *gc.C) {
	// We use two distinct directories to ensure the paths don't get
	// mixed up during the call.
	dataDir := c.MkDir()
	logDir := c.MkDir()
	info := service.NewMachineAgentInfo("0", dataDir, logDir)
	renderer, err := shell.NewRenderer("")
	c.Assert(err, jc.ErrorIsNil)
	conf := service.AgentConf(info, renderer)

	jujud := filepath.Join(dataDir, "tools", "machine-0", "jujud"+cmdSuffix)
	cmd := strings.Join([]string{
		shquote(jujud),
		"machine",
		"--data-dir", shquote(dataDir),
		"--machine-id", "0",
		"--debug",
	}, " ")
	serviceBinary := jujud
	serviceArgs := []string{
		"machine",
		"--data-dir", dataDir,
		"--machine-id", "0",
		"--debug",
	}
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:      "juju agent for machine-0",
		ExecStart: cmd,
		Logfile:   filepath.Join(logDir, "machine-0.log"),
		Env:       osenv.FeatureFlags(),
		Limit: map[string]int{
			"nofile": 20000,
		},
		Timeout:       300,
		ServiceBinary: serviceBinary,
		ServiceArgs:   serviceArgs,
	})
}

func (*agentSuite) TestAgentConfMachineUbuntu(c *gc.C) {
	dataDir := "/var/lib/juju"
	logDir := "/var/log/juju"
	info := service.NewMachineAgentInfo("0", dataDir, logDir)
	renderer, err := shell.NewRenderer("ubuntu")
	c.Assert(err, jc.ErrorIsNil)
	conf := service.AgentConf(info, renderer)

	jujud := dataDir + "/tools/machine-0/jujud"
	cmd := strings.Join([]string{
		shquote(dataDir + "/tools/machine-0/jujud"),
		"machine",
		"--data-dir", shquote(dataDir),
		"--machine-id", "0",
		"--debug",
	}, " ")
	serviceBinary := jujud
	serviceArgs := []string{
		"machine",
		"--data-dir", dataDir,
		"--machine-id", "0",
		"--debug",
	}
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:      "juju agent for machine-0",
		ExecStart: cmd,
		Logfile:   logDir + "/machine-0.log",
		Env:       osenv.FeatureFlags(),
		Limit: map[string]int{
			"nofile": 20000,
		},
		Timeout:       300,
		ServiceBinary: serviceBinary,
		ServiceArgs:   serviceArgs,
	})
}

func (*agentSuite) TestAgentConfMachineWindows(c *gc.C) {
	dataDir := `C:\Juju\lib\juju`
	logDir := `C:\Juju\logs\juju`
	info := service.NewMachineAgentInfo("0", dataDir, logDir)
	renderer, err := shell.NewRenderer("windows")
	c.Assert(err, jc.ErrorIsNil)
	conf := service.AgentConf(info, renderer)

	jujud := dataDir + `\tools\machine-0\jujud.exe`
	cmd := strings.Join([]string{
		shquote(jujud),
		"machine",
		"--data-dir", shquote(dataDir),
		"--machine-id", "0",
		"--debug",
	}, " ")
	serviceBinary := jujud
	serviceArgs := []string{
		"machine",
		"--data-dir", dataDir,
		"--machine-id", "0",
		"--debug",
	}
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:      "juju agent for machine-0",
		ExecStart: cmd,
		Logfile:   logDir + `\machine-0.log`,
		Env:       osenv.FeatureFlags(),
		Limit: map[string]int{
			"nofile": 20000,
		},
		Timeout:       300,
		ServiceBinary: serviceBinary,
		ServiceArgs:   serviceArgs,
	})
}

func (*agentSuite) TestAgentConfUnit(c *gc.C) {
	dataDir := c.MkDir()
	logDir := c.MkDir()
	info := service.NewUnitAgentInfo("wordpress/0", dataDir, logDir)
	renderer, err := shell.NewRenderer("")
	c.Assert(err, jc.ErrorIsNil)
	conf := service.AgentConf(info, renderer)

	jujud := filepath.Join(dataDir, "tools", "unit-wordpress-0", "jujud"+cmdSuffix)
	cmd := strings.Join([]string{
		shquote(jujud),
		"unit",
		"--data-dir", shquote(dataDir),
		"--unit-name", "wordpress/0",
		"--debug",
	}, " ")
	serviceBinary := jujud
	serviceArgs := []string{
		"unit",
		"--data-dir", dataDir,
		"--unit-name", "wordpress/0",
		"--debug",
	}
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:          "juju unit agent for wordpress/0",
		ExecStart:     cmd,
		Logfile:       filepath.Join(logDir, "unit-wordpress-0.log"),
		Env:           osenv.FeatureFlags(),
		Timeout:       300,
		ServiceBinary: serviceBinary,
		ServiceArgs:   serviceArgs,
	})
}

func (*agentSuite) TestContainerAgentConf(c *gc.C) {
	dataDir := c.MkDir()
	logDir := c.MkDir()
	info := service.NewUnitAgentInfo("wordpress/0", dataDir, logDir)
	renderer, err := shell.NewRenderer("")
	c.Assert(err, jc.ErrorIsNil)
	conf := service.ContainerAgentConf(info, renderer, "cont")

	jujud := filepath.Join(dataDir, "tools", "unit-wordpress-0", "jujud"+cmdSuffix)
	cmd := strings.Join([]string{
		shquote(jujud),
		"unit",
		"--data-dir", shquote(dataDir),
		"--unit-name", "wordpress/0",
		"--debug",
	}, " ")
	serviceBinary := jujud
	serviceArgs := []string{
		"unit",
		"--data-dir", dataDir,
		"--unit-name", "wordpress/0",
		"--debug",
	}
	env := osenv.FeatureFlags()
	env[osenv.JujuContainerTypeEnvKey] = "cont"
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:          "juju unit agent for wordpress/0",
		ExecStart:     cmd,
		Logfile:       filepath.Join(logDir, "unit-wordpress-0.log"),
		Env:           env,
		Timeout:       300,
		ServiceBinary: serviceBinary,
		ServiceArgs:   serviceArgs,
	})
}

func (*agentSuite) TestShutdownAfterConf(c *gc.C) {
	conf, err := service.ShutdownAfterConf("spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:         "juju shutdown job",
		Transient:    true,
		AfterStopped: "spam",
		ExecStart:    "/sbin/shutdown -h now",
	})
	renderer := &shell.BashRenderer{}
	c.Check(conf.Validate(renderer), jc.ErrorIsNil)
}

func (*agentSuite) TestShutdownAfterConfMissingServiceName(c *gc.C) {
	_, err := service.ShutdownAfterConf("")

	c.Check(err, gc.ErrorMatches, `.*missing "after" service name.*`)
}

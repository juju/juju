// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"path/filepath"
	"strings"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/shell"

	"github.com/juju/juju/internal/service"
	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/juju/osenv"
)

var (
	shquote = utils.ShQuote
)

type agentSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&agentSuite{})

func (*agentSuite) TestAgentConfMachineLocal(c *tc.C) {
	// We use two distinct directories to ensure the paths don't get
	// mixed up during the call.
	dataDir := c.MkDir()
	logDir := c.MkDir()
	info := service.NewMachineAgentInfo("0", dataDir, logDir)
	renderer, err := shell.NewRenderer("")
	c.Assert(err, jc.ErrorIsNil)
	conf := service.AgentConf(info, renderer)

	jujud := filepath.Join(dataDir, "tools", "machine-0", "jujud")
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
		Desc:          "juju agent for machine-0",
		ExecStart:     cmd,
		Logfile:       filepath.Join(logDir, "machine-0.log"),
		Env:           osenv.FeatureFlags(),
		Limit:         expectedLimits,
		Timeout:       300,
		ServiceBinary: serviceBinary,
		ServiceArgs:   serviceArgs,
	})
}

func (*agentSuite) TestAgentConfMachineUbuntu(c *tc.C) {
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
		Desc:          "juju agent for machine-0",
		ExecStart:     cmd,
		Logfile:       logDir + "/machine-0.log",
		Env:           osenv.FeatureFlags(),
		Limit:         expectedLimits,
		Timeout:       300,
		ServiceBinary: serviceBinary,
		ServiceArgs:   serviceArgs,
	})
}

func (*agentSuite) TestAgentConfUnit(c *tc.C) {
	dataDir := c.MkDir()
	logDir := c.MkDir()
	info := service.NewUnitAgentInfo("wordpress/0", dataDir, logDir)
	renderer, err := shell.NewRenderer("")
	c.Assert(err, jc.ErrorIsNil)
	conf := service.AgentConf(info, renderer)

	jujud := filepath.Join(dataDir, "tools", "unit-wordpress-0", "jujud")
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

func (*agentSuite) TestContainerAgentConf(c *tc.C) {
	dataDir := c.MkDir()
	logDir := c.MkDir()
	info := service.NewUnitAgentInfo("wordpress/0", dataDir, logDir)
	renderer, err := shell.NewRenderer("")
	c.Assert(err, jc.ErrorIsNil)
	conf := service.ContainerAgentConf(info, renderer, "cont")

	jujud := filepath.Join(dataDir, "tools", "unit-wordpress-0", "jujud")
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

func (*agentSuite) TestShutdownAfterConf(c *tc.C) {
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

func (*agentSuite) TestShutdownAfterConfMissingServiceName(c *tc.C) {
	_, err := service.ShutdownAfterConf("")

	c.Check(err, tc.ErrorMatches, `.*missing "after" service name.*`)
}

var expectedLimits = map[string]string{
	"nofile": "64000", // open files
}

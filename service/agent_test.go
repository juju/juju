// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package service_test

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

func init() {
	if runtime.GOOS == "windows" {
		cmdSuffix = ".exe"
	}
}

var cmdSuffix string

type agentSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&agentSuite{})

func (*agentSuite) TestMachineAgentConf(c *gc.C) {
	dataDir := c.MkDir()
	logDir := c.MkDir()
	conf, toolsDir := service.MachineAgentConf("0", dataDir, logDir, "")

	c.Check(toolsDir, gc.Equals, filepath.Join(dataDir, "tools", "machine-0"))
	cmd := strings.Join([]string{
		filepath.Join(toolsDir, "jujud"+cmdSuffix),
		"machine",
		"--data-dir", "'" + dataDir + "'",
		"--machine-id", "0",
		"--debug",
	}, " ")
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:      "juju agent for machine-0",
		ExecStart: cmd,
		Out:       filepath.Join(logDir, "machine-0.log"),
		Env:       osenv.FeatureFlags(),
		Limit: map[string]string{
			"nofile": "20000 20000",
		},
	})
}

func (*agentSuite) TestUnitAgentConf(c *gc.C) {
	dataDir := c.MkDir()
	logDir := c.MkDir()
	conf, toolsDir := service.UnitAgentConf("wordpress/0", dataDir, logDir, "", "cont")

	c.Check(toolsDir, gc.Equals, filepath.Join(dataDir, "tools", "unit-wordpress-0"))
	cmd := strings.Join([]string{
		filepath.Join(toolsDir, "jujud"+cmdSuffix),
		"unit",
		"--data-dir", "'" + dataDir + "'",
		"--unit-name", "wordpress/0",
		"--debug",
	}, " ")
	env := osenv.FeatureFlags()
	env[osenv.JujuContainerTypeEnvKey] = "cont"
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:      "juju unit agent for wordpress/0",
		ExecStart: cmd,
		Out:       filepath.Join(logDir, "unit-wordpress-0.log"),
		Env:       env,
	})
}

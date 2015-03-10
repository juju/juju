// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package service_test

import (
	"path"
	"path/filepath"
	"runtime"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

func init() {
	quote = "'"
	if runtime.GOOS == "windows" {
		cmdSuffix = ".exe"
		quote = `"`
	}
}

var quote, cmdSuffix string

type agentSuite struct {
	service.BaseSuite
}

var _ = gc.Suite(&agentSuite{})

func (*agentSuite) TestMachineAgentConfLocal(c *gc.C) {
	// We use two distinct directories to ensure the paths don't get
	// mixed up during the call.
	dataDir := c.MkDir()
	logDir := c.MkDir()
	conf, toolsDir := service.MachineAgentConf("0", dataDir, logDir, "")

	c.Check(toolsDir, gc.Equals, filepath.Join(dataDir, "tools", "machine-0"))
	cmd := strings.Join([]string{
		quote + filepath.Join(toolsDir, "jujud"+cmdSuffix) + quote,
		"machine",
		"--data-dir", quote + dataDir + quote,
		"--machine-id", "0",
		"--debug",
	}, " ")
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:      "juju agent for machine-0",
		ExecStart: cmd,
		Logfile:   filepath.Join(logDir, "machine-0.log"),
		Env:       osenv.FeatureFlags(),
		Limit: map[string]int{
			"nofile": 20000,
		},
		Timeout: 300,
	})
}

func (*agentSuite) TestMachineAgentConfUbuntu(c *gc.C) {
	dataDir := "/var/lib/juju"
	logDir := "/var/log/juju"
	conf, toolsDir := service.MachineAgentConf("0", dataDir, logDir, "ubuntu")

	c.Check(toolsDir, gc.Equals, dataDir+"/tools/machine-0")
	cmd := strings.Join([]string{
		"'" + toolsDir + "/jujud'",
		"machine",
		"--data-dir", "'" + dataDir + "'",
		"--machine-id", "0",
		"--debug",
	}, " ")
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:      "juju agent for machine-0",
		ExecStart: cmd,
		Logfile:   logDir + "/machine-0.log",
		Env:       osenv.FeatureFlags(),
		Limit: map[string]int{
			"nofile": 20000,
		},
		Timeout: 300,
	})
}

func (*agentSuite) TestMachineAgentConfWindows(c *gc.C) {
	dataDir := `C:\Juju\lib\juju`
	logDir := `C:\Juju\logs\juju`
	conf, toolsDir := service.MachineAgentConf("0", dataDir, logDir, "windows")

	c.Check(toolsDir, gc.Equals, dataDir+`\tools\machine-0`)
	cmd := strings.Join([]string{
		`'` + toolsDir + `\jujud.exe'`,
		"machine",
		"--data-dir", `'` + dataDir + `'`,
		"--machine-id", "0",
		"--debug",
	}, " ")
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:      "juju agent for machine-0",
		ExecStart: cmd,
		Logfile:   logDir + `\machine-0.log`,
		Env:       osenv.FeatureFlags(),
		Limit: map[string]int{
			"nofile": 20000,
		},
		Timeout: 300,
	})
}

func (*agentSuite) TestUnitAgentConf(c *gc.C) {
	dataDir := c.MkDir()
	logDir := c.MkDir()
	conf, toolsDir := service.UnitAgentConf("wordpress/0", dataDir, logDir, "", "cont")

	c.Check(toolsDir, gc.Equals, path.Join(dataDir, "tools", "unit-wordpress-0"))
	cmd := strings.Join([]string{
		quote + filepath.Join(toolsDir, "jujud"+cmdSuffix) + quote,
		"unit",
		"--data-dir", quote + dataDir + quote,
		"--unit-name", "wordpress/0",
		"--debug",
	}, " ")
	env := osenv.FeatureFlags()
	env[osenv.JujuContainerTypeEnvKey] = "cont"
	c.Check(conf, jc.DeepEquals, common.Conf{
		Desc:      "juju unit agent for wordpress/0",
		ExecStart: cmd,
		Logfile:   filepath.Join(logDir, "unit-wordpress-0.log"),
		Env:       env,
		Timeout:   300,
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
	c.Check(conf.Validate(), jc.ErrorIsNil)
}

func (*agentSuite) TestShutdownAfterConfMissingServiceName(c *gc.C) {
	_, err := service.ShutdownAfterConf("")

	c.Check(err, gc.ErrorMatches, `.*missing "after" service name.*`)
}

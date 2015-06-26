// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type formatSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&formatSuite{})

// The agentParams are used by the specific formatter whitebox tests, and is
// located here for easy reuse.
var agentParams = AgentConfigParams{
	Tag:               names.NewMachineTag("1"),
	UpgradedToVersion: version.Current.Number,
	Jobs:              []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
	Password:          "sekrit",
	CACert:            "ca cert",
	StateAddresses:    []string{"localhost:1234"},
	APIAddresses:      []string{"localhost:1235"},
	Nonce:             "a nonce",
	PreferIPv6:        false,
	Environment:       testing.EnvironmentTag,
}

func newTestConfig(c *gc.C) *configInternal {
	params := agentParams
	params.DataDir = c.MkDir()
	params.LogDir = c.MkDir()
	config, err := NewAgentConfig(params)
	c.Assert(err, jc.ErrorIsNil)
	return config.(*configInternal)
}

func (*formatSuite) TestWriteCommands(c *gc.C) {
	cloudcfg, err := cloudinit.New("quantal")
	c.Assert(err, jc.ErrorIsNil)
	config := newTestConfig(c)
	commands, err := config.WriteCommands(cloudcfg.ShellRenderer())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(commands, gc.HasLen, 3)
	c.Assert(commands[0], gc.Matches, `mkdir -p '\S+/agents/machine-1'`)
	c.Assert(commands[1], gc.Matches, `cat > '\S+/agents/machine-1/agent.conf' << 'EOF'\n(.|\n)*\nEOF`)
	c.Assert(commands[2], gc.Matches, `chmod 0600 '\S+/agents/machine-1/agent.conf'`)
}

func (*formatSuite) TestWindowsWriteCommands(c *gc.C) {
	cloudcfg, err := cloudinit.New("win8")
	c.Assert(err, jc.ErrorIsNil)
	config := newTestConfig(c)
	commands, err := config.WriteCommands(cloudcfg.ShellRenderer())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(commands, gc.HasLen, 2)
	c.Assert(commands[0], gc.Matches, `mkdir '\S+\\agents\\machine-1'`)
	c.Assert(commands[1], gc.Matches, `Set-Content '\S+/agents/machine-1/agent.conf' @"
(.|\n)*
"@`)
}

func (*formatSuite) TestWriteAgentConfig(c *gc.C) {
	config := newTestConfig(c)
	err := config.Write()
	c.Assert(err, jc.ErrorIsNil)

	configPath := ConfigPath(config.DataDir(), config.Tag())
	formatPath := filepath.Join(config.Dir(), legacyFormatFilename)
	assertFileExists(c, configPath)
	assertFileNotExist(c, formatPath)
}

func (*formatSuite) TestRead(c *gc.C) {
	config := newTestConfig(c)
	assertWriteAndRead(c, config)
}

func (*formatSuite) TestReadWriteStateConfig(c *gc.C) {
	servingInfo := params.StateServingInfo{
		Cert:         "some special cert",
		PrivateKey:   "a special key",
		CAPrivateKey: "ca special key",
		StatePort:    12345,
		APIPort:      23456,
	}
	params := agentParams
	params.DataDir = c.MkDir()
	params.Values = map[string]string{"foo": "bar", "wibble": "wobble"}
	configInterface, err := NewStateMachineConfig(params, servingInfo)
	c.Assert(err, jc.ErrorIsNil)
	config, ok := configInterface.(*configInternal)
	c.Assert(ok, jc.IsTrue)

	assertWriteAndRead(c, config)
}

func assertWriteAndRead(c *gc.C, config *configInternal) {
	err := config.Write()
	c.Assert(err, jc.ErrorIsNil)
	configPath := ConfigPath(config.DataDir(), config.Tag())
	readConfig, err := ReadConfig(configPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readConfig, jc.DeepEquals, config)
}

func assertFileExists(c *gc.C, path string) {
	fileInfo, err := os.Stat(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fileInfo.Mode().IsRegular(), jc.IsTrue)

	// Windows is not fully POSIX compliant. Chmod() and Chown() have unexpected behavior
	// compared to linux/unix
	if runtime.GOOS != "windows" {
		c.Assert(fileInfo.Mode().Perm(), gc.Equals, os.FileMode(0600))
	}
	c.Assert(fileInfo.Size(), jc.GreaterThan, 0)
}

func assertFileNotExist(c *gc.C, path string) {
	_, err := os.Stat(path)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

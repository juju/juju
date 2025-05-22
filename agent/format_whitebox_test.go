// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"
	"runtime"
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
	"github.com/juju/juju/internal/testing"
)

type formatSuite struct {
	testing.BaseSuite
}

func TestFormatSuite(t *stdtesting.T) {
	tc.Run(t, &formatSuite{})
}

// The agentParams are used by the specific formatter whitebox tests, and is
// located here for easy reuse.
var agentParams = AgentConfigParams{
	Tag:               names.NewMachineTag("1"),
	UpgradedToVersion: jujuversion.Current,
	Jobs:              []model.MachineJob{model.JobHostUnits},
	Password:          "sekrit",
	CACert:            "ca cert",
	APIAddresses:      []string{"localhost:1235"},
	Nonce:             "a nonce",
	Model:             testing.ModelTag,
	Controller:        testing.ControllerTag,
}

func newTestConfig(c *tc.C) *configInternal {
	params := agentParams
	params.Paths.DataDir = c.MkDir()
	params.Paths.LogDir = c.MkDir()
	config, err := NewAgentConfig(params)
	c.Assert(err, tc.ErrorIsNil)
	return config.(*configInternal)
}

func (*formatSuite) TestWriteCommands(c *tc.C) {
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, tc.ErrorIsNil)
	config := newTestConfig(c)
	commands, err := config.WriteCommands(cloudcfg.ShellRenderer())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(commands, tc.HasLen, 3)
	c.Assert(commands[0], tc.Matches, `mkdir -p '\S+/agents/machine-1'`)
	c.Assert(commands[1], tc.Matches, `cat > '\S+/agents/machine-1/agent.conf' << 'EOF'\n(.|\n)*\nEOF`)
	c.Assert(commands[2], tc.Matches, `chmod 0600 '\S+/agents/machine-1/agent.conf'`)
}

func (*formatSuite) TestWriteAgentConfig(c *tc.C) {
	config := newTestConfig(c)
	err := config.Write()
	c.Assert(err, tc.ErrorIsNil)

	configPath := ConfigPath(config.DataDir(), config.Tag())
	formatPath := filepath.Join(config.Dir(), "format")
	assertFileExists(c, configPath)
	assertFileNotExist(c, formatPath)
}

func (*formatSuite) TestRead(c *tc.C) {
	config := newTestConfig(c)
	assertWriteAndRead(c, config)
}

func (*formatSuite) TestReadWriteStateConfig(c *tc.C) {
	servingInfo := controller.StateServingInfo{
		Cert:         "some special cert",
		PrivateKey:   "a special key",
		CAPrivateKey: "ca special key",
		StatePort:    12345,
		APIPort:      23456,
	}
	params := agentParams
	params.Paths.DataDir = c.MkDir()
	params.Values = map[string]string{"foo": "bar", "wibble": "wobble"}
	configInterface, err := NewStateMachineConfig(params, servingInfo)
	c.Assert(err, tc.ErrorIsNil)
	config, ok := configInterface.(*configInternal)
	c.Assert(ok, tc.IsTrue)

	assertWriteAndRead(c, config)
}

func assertWriteAndRead(c *tc.C, config *configInternal) {
	err := config.Write()
	c.Assert(err, tc.ErrorIsNil)
	configPath := ConfigPath(config.DataDir(), config.Tag())
	readConfig, err := ReadConfig(configPath)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(readConfig, tc.DeepEquals, config)
}

func assertFileExists(c *tc.C, path string) {
	fileInfo, err := os.Stat(path)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fileInfo.Mode().IsRegular(), tc.IsTrue)

	// Windows is not fully POSIX compliant. Chmod() and Chown() have unexpected behavior
	// compared to linux/unix
	if runtime.GOOS != "windows" {
		c.Assert(fileInfo.Mode().Perm(), tc.Equals, os.FileMode(0600))
	}
	c.Assert(fileInfo.Size(), tc.GreaterThan, 0)
}

func assertFileNotExist(c *tc.C, path string) {
	_, err := os.Stat(path)
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}

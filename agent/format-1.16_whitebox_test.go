// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The format tests are white box tests, meaning that the tests are in the
// same package as the code, as all the format details are internal to the
// package.

package agent

import (
	"os"
	"path"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type format_1_16Suite struct {
	testbase.LoggingSuite
	formatter formatter_1_16
}

var _ = gc.Suite(&format_1_16Suite{})

func (s *format_1_16Suite) TestWriteAgentConfig(c *gc.C) {
	config := newTestConfig(c)
	err := s.formatter.write(config)
	c.Assert(err, gc.IsNil)

	expectedLocation := path.Join(config.Dir(), "agent.conf")
	fileInfo, err := os.Stat(expectedLocation)
	c.Assert(err, gc.IsNil)
	c.Assert(fileInfo.Mode().IsRegular(), jc.IsTrue)
	c.Assert(fileInfo.Mode().Perm(), gc.Equals, os.FileMode(0600))
	c.Assert(fileInfo.Size(), jc.GreaterThan, 0)

	formatLocation := path.Join(config.Dir(), formatFilename)
	fileInfo, err = os.Stat(formatLocation)
	c.Assert(err, gc.IsNil)
	c.Assert(fileInfo.Mode().IsRegular(), jc.IsTrue)
	c.Assert(fileInfo.Mode().Perm(), gc.Equals, os.FileMode(0644))
	c.Assert(fileInfo.Size(), jc.GreaterThan, 0)

	formatContent, err := readFormat(config.Dir())
	c.Assert(formatContent, gc.Equals, format_1_16)
}

func (s *format_1_16Suite) assertWriteAndRead(c *gc.C, config *configInternal) {
	err := s.formatter.write(config)
	c.Assert(err, gc.IsNil)
	// The readConfig is missing the dataDir initially.
	readConfig, err := s.formatter.read(config.Dir())
	c.Assert(err, gc.IsNil)
	c.Assert(readConfig.dataDir, gc.Equals, "")
	// This is put in by the ReadConf method that we are avoiding using
	// becuase it will have side-effects soon around migrating configs.
	readConfig.dataDir = config.dataDir
	c.Assert(readConfig, gc.DeepEquals, config)
}

func (s *format_1_16Suite) TestRead(c *gc.C) {
	config := newTestConfig(c)
	s.assertWriteAndRead(c, config)
}

func (s *format_1_16Suite) TestWriteCommands(c *gc.C) {
	config := newTestConfig(c)
	commands, err := s.formatter.writeCommands(config)
	c.Assert(err, gc.IsNil)
	c.Assert(commands, gc.HasLen, 5)
	c.Assert(commands[0], gc.Matches, `mkdir -p '\S+/agents/omg'`)
	c.Assert(commands[1], gc.Matches, `install -m 644 /dev/null '\S+/agents/omg/format'`)
	c.Assert(commands[2], gc.Matches, `printf '%s\\n' '.*' > '\S+/agents/omg/format'`)
	c.Assert(commands[3], gc.Matches, `install -m 600 /dev/null '\S+/agents/omg/agent.conf'`)
	c.Assert(commands[4], gc.Matches, `printf '%s\\n' '(.|\n)*' > '\S+/agents/omg/agent.conf'`)
}

func (s *format_1_16Suite) TestReadWriteStateConfig(c *gc.C) {
	stateParams := StateMachineConfigParams{
		AgentConfigParams: agentParams,
		StateServerCert:   []byte("some special cert"),
		StateServerKey:    []byte("a special key"),
		StatePort:         12345,
		APIPort:           23456,
	}
	stateParams.DataDir = c.MkDir()
	stateParams.Values = map[string]string{"foo": "bar", "wibble": "wobble"}
	configInterface, err := NewStateMachineConfig(stateParams)
	c.Assert(err, gc.IsNil)
	config, ok := configInterface.(*configInternal)
	c.Assert(ok, jc.IsTrue)

	s.assertWriteAndRead(c, config)
}

func (s *format_1_16Suite) TestMigrate(c *gc.C) {
	s.PatchEnvironment(JujuLxcBridge, "lxc bridge")
	s.PatchEnvironment(JujuProviderType, "provider type")
	s.PatchEnvironment(osenv.JujuContainerTypeEnvKey, "container type")
	s.PatchEnvironment(JujuStorageDir, "storage dir")
	s.PatchEnvironment(JujuStorageAddr, "storage addr")
	s.PatchEnvironment(JujuSharedStorageDir, "shared storage dir")
	s.PatchEnvironment(JujuSharedStorageAddr, "shared storage addr")

	config := newTestConfig(c)
	s.formatter.migrate(config)

	expected := map[string]string{
		LxcBridge:         "lxc bridge",
		ProviderType:      "provider type",
		ContainerType:     "container type",
		StorageDir:        "storage dir",
		StorageAddr:       "storage addr",
		SharedStorageDir:  "shared storage dir",
		SharedStorageAddr: "shared storage addr",
	}

	c.Assert(config.values, gc.DeepEquals, expected)
}

func (s *format_1_16Suite) TestMigrateOnlySetsExisting(c *gc.C) {
	s.PatchEnvironment(JujuProviderType, "provider type")

	config := newTestConfig(c)
	s.formatter.migrate(config)

	expected := map[string]string{
		ProviderType: "provider type",
	}

	c.Assert(config.values, gc.DeepEquals, expected)
}

// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type format_1_12Suite struct {
	testbase.LoggingSuite
	formatter formatter_1_12
}

var _ = gc.Suite(&format_1_12Suite{})

func newTestConfig(c *gc.C) *configInternal {
	params := agentParams
	params.DataDir = c.MkDir()
	config, err := NewAgentConfig(params)
	c.Assert(err, gc.IsNil)
	return config.(*configInternal)
}

func (s *format_1_12Suite) TestWriteAgentConfig(c *gc.C) {
	config := newTestConfig(c)
	err := s.formatter.write(config)
	c.Assert(err, gc.IsNil)

	expectedLocation := path.Join(config.Dir(), "agent.conf")
	fileInfo, err := os.Stat(expectedLocation)
	c.Assert(err, gc.IsNil)
	c.Assert(fileInfo.Mode().IsRegular(), jc.IsTrue)
	c.Assert(fileInfo.Mode().Perm(), gc.Equals, os.FileMode(0600))
	c.Assert(fileInfo.Size(), jc.GreaterThan, 0)
}

func (s *format_1_12Suite) assertWriteAndRead(c *gc.C, config *configInternal) {
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

func (s *format_1_12Suite) TestRead(c *gc.C) {
	config := newTestConfig(c)
	s.assertWriteAndRead(c, config)
}

func (s *format_1_12Suite) TestWriteCommands(c *gc.C) {
	config := newTestConfig(c)
	commands, err := s.formatter.writeCommands(config)
	c.Assert(err, gc.IsNil)
	c.Assert(commands, gc.HasLen, 3)
	c.Assert(commands[0], gc.Matches, `mkdir -p '\S+/agents/omg'`)
	c.Assert(commands[1], gc.Matches, `install -m 600 /dev/null '\S+/agents/omg/agent.conf'`)
	c.Assert(commands[2], gc.Matches, `printf '%s\\n' '(.|\n)*' > '\S+/agents/omg/agent.conf'`)
}

func (s *format_1_12Suite) TestReadWriteStateConfig(c *gc.C) {
	stateParams := StateMachineConfigParams{
		AgentConfigParams: agentParams,
		StateServerCert:   []byte("some special cert"),
		StateServerKey:    []byte("a special key"),
		StatePort:         12345,
		APIPort:           23456,
	}
	stateParams.DataDir = c.MkDir()
	configInterface, err := NewStateMachineConfig(stateParams)
	c.Assert(err, gc.IsNil)
	config, ok := configInterface.(*configInternal)
	c.Assert(ok, jc.IsTrue)

	s.assertWriteAndRead(c, config)
}

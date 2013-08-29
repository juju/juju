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

	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type format112Suite struct {
	testing.LoggingSuite
	formatter formatter112
}

var _ = gc.Suite(&format112Suite{})

var agentParams = AgentConfigParams{
	Tag:            "omg",
	Password:       "sekrit",
	CACert:         []byte("ca cert"),
	StateAddresses: []string{"localhost:1234"},
	APIAddresses:   []string{"localhost:1235"},
	Nonce:          "a nonce",
}

func (s *format112Suite) newConfig(c *gc.C) *configInternal {
	params := agentParams
	params.DataDir = c.MkDir()
	config, err := newConfig(params)
	c.Assert(err, gc.IsNil)
	return config
}

func (s *format112Suite) TestWriteAgentConfig(c *gc.C) {
	config := s.newConfig(c)
	err := s.formatter.write(config)
	c.Assert(err, gc.IsNil)

	expectedLocation := path.Join(config.Dir(), "agent.conf")
	fileInfo, err := os.Stat(expectedLocation)
	c.Assert(err, gc.IsNil)
	c.Assert(fileInfo.Mode().IsRegular(), jc.IsTrue)
	c.Assert(fileInfo.Mode().Perm(), gc.Equals, os.FileMode(0600))
	c.Assert(fileInfo.Size(), jc.GreaterThan, 0)
}

func (s *format112Suite) assertWriteAndRead(c *gc.C, config *configInternal) {
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

func (s *format112Suite) TestRead(c *gc.C) {
	config := s.newConfig(c)
	s.assertWriteAndRead(c, config)
}

func (s *format112Suite) TestWriteCommands(c *gc.C) {
	config := s.newConfig(c)
	commands, err := s.formatter.writeCommands(config)
	c.Assert(err, gc.IsNil)
	c.Assert(commands, gc.HasLen, 3)
	c.Assert(commands[0], gc.Matches, `mkdir -p '\S+/agents/omg'`)
	c.Assert(commands[1], gc.Matches, `install -m 600 /dev/null '\S+/agents/omg/agent.conf'`)
	c.Assert(commands[2], gc.Matches, `printf '%s\\n' '(.|\n)*' > '\S+/agents/omg/agent.conf'`)
}

func (s *format112Suite) TestReadWriteStateConfig(c *gc.C) {
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

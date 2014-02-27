// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The format tests are white box tests, meaning that the tests are in the
// same package as the code, as all the format details are internal to the
// package.

package agent

import (
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type format_1_18Suite struct {
	testbase.LoggingSuite
	formatter formatter_1_18
}

var _ = gc.Suite(&format_1_18Suite{})

func (s *format_1_18Suite) TestWriteAgentConfig(c *gc.C) {
	config := newTestConfig(c)
	err := s.formatter.write(config)
	c.Assert(err, gc.IsNil)

	expectedLocation := ConfigPath(config.DataDir(), config.Tag())
	fileInfo, err := os.Stat(expectedLocation)
	c.Assert(err, gc.IsNil)
	c.Assert(fileInfo.Mode().IsRegular(), jc.IsTrue)
	c.Assert(fileInfo.Mode().Perm(), gc.Equals, os.FileMode(0600))
	c.Assert(fileInfo.Size(), jc.GreaterThan, 0)

	// Make sure no format file is written.
	formatLocation := filepath.Join(config.Dir(), legacyFormatFilename)
	_, err = os.Stat(formatLocation)
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *format_1_18Suite) assertWriteAndRead(c *gc.C, config *configInternal) {
	err := s.formatter.write(config)
	c.Assert(err, gc.IsNil)
	readConfig, err := s.formatter.read(ConfigPath(config.DataDir(), config.Tag()))
	c.Assert(err, gc.IsNil)
	// logDir is empty to it gets the default.
	readConfig.logDir = config.logDir
	c.Assert(readConfig, jc.DeepEquals, config)
}

func (s *format_1_18Suite) TestRead(c *gc.C) {
	config := newTestConfig(c)
	s.assertWriteAndRead(c, config)
}

func (s *format_1_18Suite) TestWriteCommands(c *gc.C) {
	config := newTestConfig(c)
	commands, err := s.formatter.writeCommands(config)
	c.Assert(err, gc.IsNil)
	c.Assert(commands, gc.HasLen, 3)
	c.Assert(commands[0], gc.Matches, `mkdir -p '\S+/agents/omg'`)
	c.Assert(commands[1], gc.Matches, `install -m 600 /dev/null '\S+/agents/omg/agent.conf'`)
	c.Assert(commands[2], gc.Matches, `printf '%s\\n' '(.|\n)*' > '\S+/agents/omg/agent.conf'`)
}

func (s *format_1_18Suite) TestReadWriteStateConfig(c *gc.C) {
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

func (s *format_1_18Suite) TestMigrate(c *gc.C) {
	config := newTestConfig(c)
	config.logDir = ""
	s.formatter.migrate(config)

	// LogDir is set only when empty.
	c.Assert(config.logDir, gc.Equals, DefaultLogDir)
	config.logDir = "/path/log"

	s.formatter.migrate(config)
	c.Assert(config.logDir, gc.Equals, "/path/log")
}

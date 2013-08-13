// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type DebugLogSuite struct {
}

var _ = gc.Suite(&DebugLogSuite{})

func runDebugLog(c *gc.C, args ...string) (*DebugLogCommand, error) {
	cmd := &DebugLogCommand{
		sshCmd: &dummySSHCommand{},
	}
	_, err := testing.RunCommand(c, cmd, args)
	return cmd, err
}

type dummySSHCommand struct {
	SSHCommand
	runCalled bool
}

func (c *dummySSHCommand) Run(ctx *cmd.Context) error {
	c.runCalled = true
	return nil
}

// debug-log is implemented by invoking juju ssh with the correct arguments.
// This test helper checks for the expected invocation.
func (s *DebugLogSuite) assertDebugLogInvokesSSHCommand(c *gc.C, expected string, args ...string) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	debugLogCmd, err := runDebugLog(c, args...)
	c.Assert(err, gc.IsNil)
	debugCmd := debugLogCmd.sshCmd.(*dummySSHCommand)
	c.Assert(debugCmd.runCalled, gc.Equals, true)
	c.Assert(debugCmd.Target, gc.Equals, "0")
	c.Assert([]string{expected}, gc.DeepEquals, debugCmd.Args)
}

const logLocation = "/var/log/juju/all-machines.log"

func (s *DebugLogSuite) TestDebugLog(c *gc.C) {
	const expected = "tail -n 10 -f " + logLocation
	s.assertDebugLogInvokesSSHCommand(c, expected)
}

func (s *DebugLogSuite) TestDebugLogFrom(c *gc.C) {
	const expected = "tail -n +1 -f " + logLocation
	s.assertDebugLogInvokesSSHCommand(c, expected, "-n", "+1")
	s.assertDebugLogInvokesSSHCommand(c, expected, "--lines=+1")
}

func (s *DebugLogSuite) TestDebugLogLast(c *gc.C) {
	const expected = "tail -n 100 -f " + logLocation
	s.assertDebugLogInvokesSSHCommand(c, expected, "-n", "100")
	s.assertDebugLogInvokesSSHCommand(c, expected, "--lines=100")
}

func (s *DebugLogSuite) TestDebugLogValidation(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	_, err := runDebugLog(c, "-n", "0")
	c.Assert(err, gc.ErrorMatches, "invalid value \"0\" for flag -n: invalid number of lines")
	_, err = runDebugLog(c, "-n", "-1")
	c.Assert(err, gc.ErrorMatches, "invalid value \"-1\" for flag -n: invalid number of lines")
	_, err = runDebugLog(c, "-n", "fnord")
	c.Assert(err, gc.ErrorMatches, "invalid value \"fnord\" for flag -n: invalid number of lines")
}

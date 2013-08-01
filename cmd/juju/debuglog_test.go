// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type DebugLogSuite struct {
}

var _ = Suite(&DebugLogSuite{})

func runDebugLog(c *C, args ...string) (*DebugLogCommand, error) {
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
// This test checks for the expected invocation.
func (s *DebugLogSuite) TestDebugLogInvokesSSHCommand(c *C) {
	defer testing.MakeEmptyFakeHome(c).Restore()
	debugLogCmd, err := runDebugLog(c)
	c.Assert(err, IsNil)
	debugCmd := debugLogCmd.sshCmd.(*dummySSHCommand)
	c.Assert(debugCmd.runCalled, Equals, true)
	c.Assert(debugCmd.Target, Equals, "0")
	c.Assert([]string{"tail -f /var/log/juju/all-machines.log"}, DeepEquals, debugCmd.Args)

	debugLogCmd, err = runDebugLog(c, "--all")
	c.Assert(err, IsNil)
	debugCmd = debugLogCmd.sshCmd.(*dummySSHCommand)
	c.Assert(debugCmd.runCalled, Equals, true)
	c.Assert(debugCmd.Target, Equals, "0")
	c.Assert([]string{"tail -n +1 -f /var/log/juju/all-machines.log"}, DeepEquals, debugCmd.Args)
}

package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type DebugLogSuite struct {
}

var _ = Suite(&DebugLogSuite{})

func runDebugLog(c *C, args ...string) error {
	_, err := testing.RunCommand(c, &DebugLogCommand{}, args)
	return err
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
	debugLogSSHCmd = &dummySSHCommand{}
	err := runDebugLog(c)
	c.Assert(err, IsNil)
	debugCmd := debugLogSSHCmd.(*dummySSHCommand)
	c.Assert(debugCmd.runCalled, Equals, true)
	c.Assert(debugCmd.Target, Equals, "0")
	c.Assert([]string{"tail -f /var/log/juju/all-machines.log"}, DeepEquals, debugCmd.Args)
}

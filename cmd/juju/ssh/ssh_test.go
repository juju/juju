// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type CmdSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CmdSuite{})

func initSSHCommand(args ...string) (*sshCommand, error) {
	com := &sshCommand{}
	return com, cmdtesting.InitCommand(com, args)
}

func (*CmdSuite) TestSSHCommandInit(c *gc.C) {
	// missing args
	_, err := initSSHCommand()
	c.Assert(err, gc.ErrorMatches, "no target name specified")
}

func initSCPCommand(args ...string) (*scpCommand, error) {
	com := &scpCommand{}
	return com, cmdtesting.InitCommand(com, args)
}

func (*CmdSuite) TestSCPCommandInit(c *gc.C) {
	// missing args
	_, err := initSCPCommand()
	c.Assert(err, gc.ErrorMatches, "at least two arguments required")

	// not enough args
	_, err = initSCPCommand("mysql/0:foo")
	c.Assert(err, gc.ErrorMatches, "at least two arguments required")
}

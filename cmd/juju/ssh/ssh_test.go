// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
)

type CmdSuite struct {
	testhelpers.IsolationSuite
}

func TestCmdSuite(t *stdtesting.T) { tc.Run(t, &CmdSuite{}) }
func initSSHCommand(args ...string) (*sshCommand, error) {
	com := &sshCommand{}
	return com, cmdtesting.InitCommand(com, args)
}

func (*CmdSuite) TestSSHCommandInit(c *tc.C) {
	// missing args
	_, err := initSSHCommand()
	c.Assert(err, tc.ErrorMatches, "no target name specified")
}

func initSCPCommand(args ...string) (*scpCommand, error) {
	com := &scpCommand{}
	return com, cmdtesting.InitCommand(com, args)
}

func (*CmdSuite) TestSCPCommandInit(c *tc.C) {
	// missing args
	_, err := initSCPCommand()
	c.Assert(err, tc.ErrorMatches, "at least two arguments required")

	// not enough args
	_, err = initSCPCommand("mysql/0:foo")
	c.Assert(err, tc.ErrorMatches, "at least two arguments required")
}

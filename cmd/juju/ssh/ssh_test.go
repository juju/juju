// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testhelpers"
)

type CmdSuite struct {
	testhelpers.IsolationSuite
}

func TestCmdSuite(t *testing.T) {
	tc.Run(t, &CmdSuite{})
}

func initSSHCommand(args ...string) (*sshCommand, error) {
	com := &sshCommand{}
	return com, cmdtesting.InitCommand(com, args)
}

func (*CmdSuite) TestSSHCommandInit(c *tc.C) {
	// missing args
	_, err := initSSHCommand()
	c.Assert(err, tc.ErrorMatches, "no target name specified")
}

func (*CmdSuite) TestSSHCommandInitUsesJumpProvider(c *tc.C) {
	cmd := NewSSHCommandForTest(
		nil, nil, nil, nil, nil,
		baseTestingRetryStrategy, baseTestingRetryStrategy,
	)
	cmd.SetClientStore(clientStoreWithModelType(model.IAAS))
	c.Assert(cmd.SetModelIdentifier("arthur:admin/controller", false), tc.ErrorIsNil)

	err := cmd.Init([]string{"0", "hostname"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cmd.jump, tc.IsTrue)

	provider, ok := cmd.provider.(*sshJump)
	c.Assert(ok, tc.IsTrue)
	c.Check(provider.getTarget(), tc.Equals, "0")
	c.Check(provider.getArgs(), tc.DeepEquals, []string{"hostname"})
}

func (*CmdSuite) TestSSHCommandInitUsesJumpProviderForCAAS(c *tc.C) {
	cmd := NewSSHCommandForTest(
		nil, nil, nil, nil, nil,
		baseTestingRetryStrategy, baseTestingRetryStrategy,
	)
	cmd.SetClientStore(clientStoreWithModelType(model.CAAS))
	c.Assert(cmd.SetModelIdentifier("arthur:admin/controller", false), tc.ErrorIsNil)
	cmd.sshContainer.container = "redis"

	err := cmd.Init([]string{"redis/0"})
	c.Assert(err, tc.ErrorIsNil)

	provider, ok := cmd.provider.(*sshJump)
	c.Assert(ok, tc.IsTrue)
	c.Check(provider.container, tc.Equals, "redis")
}

func (*CmdSuite) TestSSHCommandErrorsShowCommandWithDirect(c *tc.C) {
	cmd := NewSSHCommandForTest(
		nil, nil, nil, nil, nil,
		baseTestingRetryStrategy, baseTestingRetryStrategy,
	)
	cmd.SetClientStore(clientStoreWithModelType(model.IAAS))
	c.Assert(cmd.SetModelIdentifier("arthur:admin/controller", false), tc.ErrorIsNil)

	err := cmdtesting.InitCommand(cmd, []string{"--direct", "--show-command", "0"})
	c.Assert(err, tc.ErrorMatches, "--show-command cannot be used with --direct")
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

func (*CmdSuite) TestSCPCommandInitUsesJumpProvider(c *tc.C) {
	cmd := NewSCPCommandForTest(
		nil, nil, nil, nil,
		baseTestingRetryStrategy, baseTestingRetryStrategy,
	)
	cmd.SetClientStore(clientStoreWithModelType(model.IAAS))
	c.Assert(cmd.SetModelIdentifier("arthur:admin/controller", false), tc.ErrorIsNil)

	err := cmd.Init([]string{"source", "0:/tmp/source"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cmd.jump, tc.IsTrue)

	provider, ok := cmd.provider.(*sshJump)
	c.Assert(ok, tc.IsTrue)
	c.Check(provider.getArgs(), tc.DeepEquals, []string{"source", "0:/tmp/source"})
}

func (*CmdSuite) TestSCPCommandInitUsesJumpProviderForCAAS(c *tc.C) {
	cmd := NewSCPCommandForTest(
		nil, nil, nil, nil,
		baseTestingRetryStrategy, baseTestingRetryStrategy,
	)
	cmd.SetClientStore(clientStoreWithModelType(model.CAAS))
	c.Assert(cmd.SetModelIdentifier("arthur:admin/controller", false), tc.ErrorIsNil)
	cmd.sshContainer.container = "redis"

	err := cmd.Init([]string{"source", "redis/0:/tmp/source"})
	c.Assert(err, tc.ErrorIsNil)

	provider, ok := cmd.provider.(*sshJump)
	c.Assert(ok, tc.IsTrue)
	c.Check(provider.container, tc.Equals, "redis")
}

func (*CmdSuite) TestSCPCommandErrorsShowCommandWithDirect(c *tc.C) {
	cmd := NewSCPCommandForTest(
		nil, nil, nil, nil,
		baseTestingRetryStrategy, baseTestingRetryStrategy,
	)
	cmd.SetClientStore(clientStoreWithModelType(model.IAAS))
	c.Assert(cmd.SetModelIdentifier("arthur:admin/controller", false), tc.ErrorIsNil)

	err := cmdtesting.InitCommand(cmd, []string{"--direct", "--show-command", "source", "0:/tmp/source"})
	c.Assert(err, tc.ErrorMatches, "--show-command cannot be used with --direct")
}

func (*CmdSuite) TestDebugHooksCommandInitUsesJumpProvider(c *tc.C) {
	cmd := NewDebugHooksCommandForTest(
		nil, nil, nil, nil, nil,
		baseTestingRetryStrategy, baseTestingRetryStrategy,
	)
	cmd.SetClientStore(clientStoreWithModelType(model.IAAS))
	c.Assert(cmd.SetModelIdentifier("arthur:admin/controller", false), tc.ErrorIsNil)

	err := cmd.Init([]string{"mysql/0"})
	c.Assert(err, tc.ErrorIsNil)
	_, ok := cmd.provider.(*sshJump)
	c.Check(ok, tc.IsTrue)
}

func (*CmdSuite) TestDebugCodeCommandInitUsesJumpProvider(c *tc.C) {
	cmd := NewDebugCodeCommandForTest(
		nil, nil, nil, nil, nil,
		baseTestingRetryStrategy, baseTestingRetryStrategy,
	)
	cmd.SetClientStore(clientStoreWithModelType(model.IAAS))
	c.Assert(cmd.SetModelIdentifier("arthur:admin/controller", false), tc.ErrorIsNil)

	err := cmd.Init([]string{"mysql/0"})
	c.Assert(err, tc.ErrorIsNil)
	_, ok := cmd.provider.(*sshJump)
	c.Check(ok, tc.IsTrue)
}

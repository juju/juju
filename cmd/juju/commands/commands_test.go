// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
)

var _ = gc.Suite(&commandsSuite{})
var _ = gc.Suite(&commandRegistryItemSuite{})

type commandsSuite struct {
	stub    *testing.Stub
	command *stubCommand
}

func (s *commandsSuite) SetUpTest(c *gc.C) {
	s.stub = &testing.Stub{}
	s.command = &stubCommand{stub: s.stub}
}

func (s *commandsSuite) TearDownTest(c *gc.C) {
	registeredCommands = nil
}

func (s *commandsSuite) TestRegisterCommand(c *gc.C) {
	RegisterCommand(func() cmd.Command {
		return s.command
	})

	// We can't compare functions directly, so...
	c.Assert(registeredCommands, gc.HasLen, 1)
	c.Check(registeredCommands[0].newEnvCommand, gc.IsNil)
	command := registeredCommands[0].newCommand()
	c.Check(command, gc.Equals, s.command)
}

func (s *commandsSuite) TestRegisterEnvCommand(c *gc.C) {
	RegisterEnvCommand(func() envcmd.EnvironCommand {
		return s.command
	})

	// We can't compare functions directly, so...
	c.Assert(registeredCommands, gc.HasLen, 1)
	c.Check(registeredCommands[0].newCommand, gc.IsNil)
	command := registeredCommands[0].newEnvCommand()
	c.Check(command, gc.Equals, s.command)
}

type commandRegistryItemSuite struct {
	stub     *testing.Stub
	command  *stubCommand
	registry *stubRegistry
}

func (s *commandRegistryItemSuite) newCommand() cmd.Command {
	return s.command
}

func (s *commandRegistryItemSuite) newEnvCommand() envcmd.EnvironCommand {
	return s.command
}

func (s *commandRegistryItemSuite) SetUpTest(c *gc.C) {
	s.stub = &testing.Stub{}
	s.command = &stubCommand{stub: s.stub}
	s.registry = &stubRegistry{stub: s.stub}
}

func (s *commandRegistryItemSuite) TestCommandBasic(c *gc.C) {
	ctx := &cmd.Context{}
	item := commandRegistryItem{
		newCommand: s.newCommand,
	}
	command := item.command(ctx)

	c.Check(command, gc.Equals, s.command)
}

func (s *commandRegistryItemSuite) TestCommandEnv(c *gc.C) {
	ctx := &cmd.Context{}
	item := commandRegistryItem{
		newEnvCommand: s.newEnvCommand,
	}
	command := item.command(ctx)

	c.Check(command, jc.DeepEquals, envCmdWrapper{
		Command: envcmd.Wrap(s.command),
		ctx:     ctx,
	})
}

func (s *commandRegistryItemSuite) TestCommandZeroValue(c *gc.C) {
	ctx := &cmd.Context{}
	var item commandRegistryItem
	command := item.command(ctx)

	c.Check(command, gc.IsNil)
}

func (s *commandRegistryItemSuite) TestApply(c *gc.C) {
	ctx := &cmd.Context{}
	item := commandRegistryItem{
		newCommand: s.newCommand,
	}
	item.apply(s.registry, ctx)

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Register",
		Args: []interface{}{
			s.command,
		},
	}})
}

type stubCommand struct {
	cmd.CommandBase
	stub *testing.Stub
	info *cmd.Info
}

func (c *stubCommand) Info() *cmd.Info {
	c.stub.AddCall("Info")
	c.stub.NextErr() // pop one off

	return c.info
}

func (c *stubCommand) Run(ctx *cmd.Context) error {
	c.stub.AddCall("Run", ctx)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *stubCommand) SetEnvName(name string) {
	c.stub.AddCall("SetEnvName", name)
	c.stub.NextErr() // pop one off

	// Do nothing.
}

type stubRegistry struct {
	stub *testing.Stub
}

func (r *stubRegistry) Register(sub cmd.Command) {
	r.stub.AddCall("Register", sub)
	r.stub.NextErr() // pop one off

	// Do nothing.
}

func (r *stubRegistry) RegisterSuperAlias(name, super, forName string, check cmd.DeprecationCheck) {
	r.stub.AddCall("RegisterSuperAlias", name, super, forName)
	r.stub.NextErr() // pop one off

	// Do nothing.
}

func (r *stubRegistry) RegisterDeprecated(subcmd cmd.Command, check cmd.DeprecationCheck) {
	r.stub.AddCall("RegisterDeprecated", subcmd, check)
	r.stub.NextErr() // pop one off

	// Do nothing.
}

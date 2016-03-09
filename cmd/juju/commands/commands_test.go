// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
)

var _ = gc.Suite(&commandsSuite{})

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
	registeredEnvCommands = nil
}

func (s *commandsSuite) TestRegisterCommand(c *gc.C) {
	RegisterCommand(func() cmd.Command {
		return s.command
	})

	// We can't compare functions directly, so...
	c.Check(registeredEnvCommands, gc.HasLen, 0)
	c.Assert(registeredCommands, gc.HasLen, 1)
	command := registeredCommands[0]()
	c.Check(command, gc.Equals, s.command)
}

func (s *commandsSuite) TestRegisterEnvCommand(c *gc.C) {
	RegisterEnvCommand(func() modelcmd.ModelCommand {
		return s.command
	})

	// We can't compare functions directly, so...
	c.Assert(registeredCommands, gc.HasLen, 0)
	c.Assert(registeredEnvCommands, gc.HasLen, 1)
	command := registeredEnvCommands[0]()
	c.Check(command, gc.Equals, s.command)
}

type stubCommand struct {
	modelcmd.ModelCommandBase
	stub    *testing.Stub
	info    *cmd.Info
	envName string
}

func (c *stubCommand) Info() *cmd.Info {
	c.stub.AddCall("Info")
	c.stub.NextErr() // pop one off

	if c.info == nil {
		return &cmd.Info{
			Name: "some-command",
		}
	}
	return c.info
}

func (c *stubCommand) Run(ctx *cmd.Context) error {
	c.stub.AddCall("Run", ctx)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *stubCommand) SetModelName(name string) error {
	c.stub.AddCall("SetModelName", name)
	c.envName = name
	return c.stub.NextErr()
}

func (c *stubCommand) ModelName() string {
	c.stub.AddCall("ModelName")
	c.stub.NextErr() // pop one off

	return c.envName
}

type stubRegistry struct {
	stub *testing.Stub

	names []string
}

func (r *stubRegistry) Register(subcmd cmd.Command) {
	r.stub.AddCall("Register", subcmd)
	r.stub.NextErr() // pop one off

	r.names = append(r.names, subcmd.Info().Name)
	for _, name := range subcmd.Info().Aliases {
		r.names = append(r.names, name)
	}
}

func (r *stubRegistry) RegisterSuperAlias(name, super, forName string, check cmd.DeprecationCheck) {
	r.stub.AddCall("RegisterSuperAlias", name, super, forName)
	r.stub.NextErr() // pop one off

	r.names = append(r.names, name)
}

func (r *stubRegistry) RegisterDeprecated(subcmd cmd.Command, check cmd.DeprecationCheck) {
	r.stub.AddCall("RegisterDeprecated", subcmd, check)
	r.stub.NextErr() // pop one off

	r.names = append(r.names, subcmd.Info().Name)
	for _, name := range subcmd.Info().Aliases {
		r.names = append(r.names, name)
	}
}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/process"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// baseCommand implements the common portions of the workload process
// hook env commands.
type baseCommand struct {
	cmd.CommandBase

	ctx     jujuc.Context
	compCtx jujuc.ContextComponent

	// Name is the name of the process in charm metadata.
	Name string
	// info is the process info for the named workload process.
	info *process.Info
}

func newCommand(ctx jujuc.Context) baseCommand {
	compCtx, err := ContextComponent(ctx)
	if err != nil {
		// The component wasn't registered properly.
		panic(err)
	}
	return baseCommand{
		ctx:     ctx,
		compCtx: compCtx,
	}
}

// Info implements cmd.Command.
func (c *baseCommand) Info() *cmd.Info {
	panic("not implemented")
}

// Init implements cmd.Command.
func (c *baseCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("missing process name")
	}
	return errors.Trace(c.init(args[0]))
}

func (c *baseCommand) init(name string) error {
	if name == "" {
		return errors.Errorf("got empty name")
	}
	var pInfo process.Info
	if err := c.compCtx.Get(name, &pInfo); err != nil {
		return errors.Trace(err)
	}
	c.info = &pInfo
	c.Name = name
	return nil
}

// Run implements cmd.Command.
func (c *baseCommand) Run(ctx *cmd.Context) error {
	panic("not implemented")
}

// registeringCommand is the base for commands that register a process
// that has been launched.
type registeringCommand struct {
	baseCommand

	// Id is the unique ID for the launched process.
	Id string
	// Details is the launch details returned from the process plugin.
	Details process.LaunchDetails
}

func newRegisteringCommand(ctx jujuc.Context) registeringCommand {
	return registeringCommand{
		baseCommand: newCommand(ctx),
	}
}

// SetFlags implements cmd.Command.
func (c *registeringCommand) SetFlags(f *gnuflag.FlagSet) {
}

func (c *registeringCommand) init(name string) error {
	if err := c.baseCommand.init(name); err != nil {
		return errors.Trace(err)
	}
	if err := c.checkSpace(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// checkSpace ensures that the requested network space is available
// to the hook.
func (c *registeringCommand) checkSpace() error {
	// TODO(wwitzel3) implement this to ensure that the endpoints provided exist in this space
	return nil
}

// register updates the hook context with the information for the
// registered workload process. An error is returned if the process
// was already registered.
func (c *registeringCommand) register() error {
	if c.info.IsRegistered() {
		return errors.Errorf("already registered")
	}
	c.info.Details = c.Details
	if err := c.compCtx.Set(c.Name, c.info); err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) flush here?
	return nil
}

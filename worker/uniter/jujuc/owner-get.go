// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"errors"
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
)

// OwnerGetCommand implements the owner-get command.
type OwnerGetCommand struct {
	cmd.CommandBase
	ctx Context
	Key string
	out cmd.Output
}

func NewOwnerGetCommand(ctx Context) cmd.Command {
	return &OwnerGetCommand{ctx: ctx}
}

func (c *OwnerGetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "owner-get",
		Args:    "<setting>",
		Purpose: `print information about the owner of the service. The only valid value for <setting> is currently tag`,
	}
}

func (c *OwnerGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *OwnerGetCommand) Init(args []string) error {
	if args == nil {
		return errors.New("no setting specified")
	}
	if args[0] != "tag" {
		return fmt.Errorf("unknown setting %q", args[0])
	}
	c.Key = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *OwnerGetCommand) Run(ctx *cmd.Context) error {
	if c.Key != "tag" {
		return fmt.Errorf("%s not set", c.Key)
	}

	return c.out.Write(ctx, c.ctx.OwnerTag())
}

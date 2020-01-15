// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// StateValueGetCommand implements the state-value-get command.
type StateValueGetCommand struct {
	cmd.CommandBase
	ctx Context
	Key string // The key to show
	out cmd.Output
}

func NewStateValueGetCommand(ctx Context) (cmd.Command, error) {
	return &StateValueGetCommand{ctx: ctx}, nil
}

func (c *StateValueGetCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "state-value-get",
		Args:    "<key>",
		Purpose: "print server-side-state value",
	})
}

func (c *StateValueGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

func (c *StateValueGetCommand) Init(args []string) error {
	if args == nil {
		return nil
	}
	c.Key = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *StateValueGetCommand) Run(ctx *cmd.Context) error {
	value, err := c.ctx.GetStateValue(c.Key)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, value)
}

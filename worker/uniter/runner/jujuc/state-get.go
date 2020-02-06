// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// StateGetCommand implements the state-get command.
type StateGetCommand struct {
	cmd.CommandBase
	ctx Context
	Key string // The key to show
	out cmd.Output
}

// NewStateGetCommand returns a state-get command.
func NewStateGetCommand(ctx Context) (cmd.Command, error) {
	return &StateGetCommand{ctx: ctx}, nil
}

// Info returns information about the Command.
// Info implements part of the cmd.Command interface.
func (c *StateGetCommand) Info() *cmd.Info {
	doc := `
state-get prints the value of the server side state specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.

See also:
    state-delete
    state-set
`
	return jujucmd.Info(&cmd.Info{
		Name:    "state-get",
		Args:    "[<key>]",
		Purpose: "print server-side-state value",
		Doc:     doc,
	})
}

// SetFlags adds command specific flags to the flag set.
// SetFlags implements part of the cmd.Command interface.
func (c *StateGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

// Init initializes the Command before running.
// Init implements part of the cmd.Command interface.
func (c *StateGetCommand) Init(args []string) error {
	if args == nil {
		return nil
	}
	c.Key = ""
	if len(args) > 0 {
		if c.Key = args[0]; c.Key == "-" {
			c.Key = ""
		}
		args = args[1:]
	}
	return cmd.CheckEmpty(args)
}

// Run will execute the Command as directed by the options and positional
// arguments passed to Init.
// Run implements part of the cmd.Command interface.
func (c *StateGetCommand) Run(ctx *cmd.Context) error {
	if c.Key == "" {
		cache, err := c.ctx.GetCache()
		if err != nil {
			return err
		}
		return c.out.Write(ctx, cache)
	}

	value, err := c.ctx.GetSingleCacheValue(c.Key)
	if err != nil {
		return err
	}

	return c.out.Write(ctx, value)
}

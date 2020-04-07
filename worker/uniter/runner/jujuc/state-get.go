// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// StateGetCommand implements the state-get command.
type StateGetCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output

	key    string // The key to show
	strict bool
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
	f.BoolVar(&c.strict, "strict", false, "Return an error if the requested key does not exist")
}

// Init initializes the Command before running.
// Init implements part of the cmd.Command interface.
func (c *StateGetCommand) Init(args []string) error {
	if args == nil {
		return nil
	}
	c.key = ""
	if len(args) > 0 {
		if c.key = args[0]; c.key == "-" {
			c.key = ""
		}
		args = args[1:]
	}
	return cmd.CheckEmpty(args)
}

// Run will execute the Command as directed by the options and positional
// arguments passed to Init.
// Run implements part of the cmd.Command interface.
func (c *StateGetCommand) Run(ctx *cmd.Context) error {
	if c.key == "" {
		cache, err := c.ctx.GetCharmState()
		if err != nil {
			return err
		}
		return c.out.Write(ctx, cache)
	}

	value, err := c.ctx.GetCharmStateValue(c.key)
	notFound := errors.IsNotFound(err)
	if err != nil && (!notFound || (notFound && c.strict)) {
		return err
	}
	return c.out.Write(ctx, value)
}

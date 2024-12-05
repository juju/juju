// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/internal/cmd"
)

// StateDeleteCommand implements the state-delete command.
type StateDeleteCommand struct {
	cmd.CommandBase
	ctx Context
	Key string // The key to delete
}

// NewStateDeleteCommand returns a state-delete command.
func NewStateDeleteCommand(ctx Context) (cmd.Command, error) {
	return &StateDeleteCommand{ctx: ctx}, nil
}

// Info returns information about the Command.
// Info implements part of the cmd.Command interface.
func (c *StateDeleteCommand) Info() *cmd.Info {
	doc := `
state-delete deletes the value of the server side state specified by key.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "state-delete",
		Args:    "<key>",
		Purpose: "Delete server-side-state key value pairs.",
		Doc:     doc,
		SeeAlso: []string{"state-get", "state-set"},
	})
}

// Init initializes the Command before running.
// Init implements part of the cmd.Command interface.
func (c *StateDeleteCommand) Init(args []string) error {
	if args == nil {
		return nil
	}
	c.Key = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run will execute the Command as directed by the options and positional
// arguments passed to Init.
// Run implements part of the cmd.Command interface.
func (c *StateDeleteCommand) Run(ctx *cmd.Context) error {
	if c.Key == "" {
		return nil
	}
	err := c.ctx.DeleteCharmStateValue(ctx, c.Key)
	if err != nil {
		return err
	}
	return nil
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

// ActionFailCommand implements the action-fail command.
type ActionFailCommand struct {
	cmd.CommandBase
	ctx         Context
	failMessage string
}

// NewActionFailCommand returns a new ActionFailCommand with the given context.
func NewActionFailCommand(ctx Context) cmd.Command {
	return &ActionFailCommand{ctx: ctx}
}

// Info returns the content for --help.
func (c *ActionFailCommand) Info() *cmd.Info {
	doc := `
action-fail sets the action's fail state with a given error message.  Using
action-fail without a failure message will set a default message indicating a
problem with the action.
`
	return &cmd.Info{
		Name:    "action-fail",
		Args:    "[\"<failure message>\"]",
		Purpose: "set action fail status with message",
		Doc:     doc,
	}
}

// SetFlags handles any option flags, but there are none.
func (c *ActionFailCommand) SetFlags(f *gnuflag.FlagSet) {
}

// Init sets the fail message and checks for malformed invocations.
func (c *ActionFailCommand) Init(args []string) error {
	if len(args) == 0 {
		c.failMessage = "action failed without reason given, check action for errors"
		return nil
	}
	c.failMessage = args[0]
	return cmd.CheckEmpty(args[1:])
}

// Run sets the Action's fail state.
func (c *ActionFailCommand) Run(ctx *cmd.Context) error {
	err := c.ctx.SetActionMessage(c.failMessage)
	if err != nil {
		return err
	}
	return c.ctx.SetActionFailed()
}

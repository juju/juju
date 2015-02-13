// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

// IsLeaderCommand implements the is-leader command.
type IsLeaderCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output
}

// NewIsLeaderCommand returns a new IsLeaderCommand with the given context.
func NewIsLeaderCommand(ctx Context) cmd.Command {
	return &IsLeaderCommand{ctx: ctx}
}

// Info is part of the cmd.Command interface.
func (c *IsLeaderCommand) Info() *cmd.Info {
	doc := `
is-leader prints a boolean indicating whether the local unit is guaranteed to
be service leader for at least 30 seconds. If it fails, you should assume that
there is no such guarantee.
`
	return &cmd.Info{
		Name:    "is-leader",
		Purpose: "print service leadership status",
		Doc:     doc,
	}
}

// SetFlags is part of the cmd.Command interface.
func (c *IsLeaderCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

// Run is part of the cmd.Command interface.
func (c *IsLeaderCommand) Run(ctx *cmd.Context) error {
	success, err := c.ctx.IsLeader()
	if err != nil {
		return errors.Annotatef(err, "leadership status unknown")
	}
	return c.out.Write(ctx, success)
}

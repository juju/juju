// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/feature"
)

// isLeaderCommand implements the is-leader command.
type isLeaderCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output
}

// NewIsLeaderCommand returns a new isLeaderCommand with the given context.
func NewIsLeaderCommand(ctx Context) (cmd.Command, error) {
	return &isLeaderCommand{ctx: ctx}, nil
}

// Info is part of the cmd.Command interface.
func (c *isLeaderCommand) Info() *cmd.Info {
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
func (c *isLeaderCommand) SetFlags(f *gnuflag.FlagSet) {
	if featureflag.Enabled(feature.SmartFormatter) {
		c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	} else {
		c.out.AddFlags(f, "yaml", output.DefaultFormatters)
	}
}

// Run is part of the cmd.Command interface.
func (c *isLeaderCommand) Run(ctx *cmd.Context) error {
	success, err := c.ctx.IsLeader()
	if err != nil {
		return errors.Annotatef(err, "leadership status unknown")
	}
	return c.out.Write(ctx, success)
}

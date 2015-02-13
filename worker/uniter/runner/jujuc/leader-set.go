// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/keyvalues"
)

// LeaderSetCommand implements the leader-set command.
type LeaderSetCommand struct {
	cmd.CommandBase
	ctx      Context
	Settings map[string]string
}

// NewLeaderSetCommand returns a new LeaderSetCommand with the given context.
func NewLeaderSetCommand(ctx Context) cmd.Command {
	return &LeaderSetCommand{ctx: ctx}
}

// Info is part of the cmd.Command interface.
func (c *LeaderSetCommand) Info() *cmd.Info {
	doc := `
leader-set immediate writes the supplied key/value pairs to the state server,
which will then inform non-leader units of the change. It will fail if called
without arguments.
`
	return &cmd.Info{
		Name:    "leader-set",
		Args:    "<key>=<value> [...]",
		Purpose: "write service leadership settings",
		Doc:     doc,
	}
}

// Init is part of the cmd.Command interface.
func (c *LeaderSetCommand) Init(args []string) (err error) {
	c.Settings, err = keyvalues.Parse(args, true)
	return
}

// Run is part of the cmd.Command interface.
func (c *LeaderSetCommand) Run(_ *cmd.Context) error {
	err := c.ctx.WriteLeaderSettings(c.Settings)
	return errors.Annotatef(err, "cannot write leadership settings")
}

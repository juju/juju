// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/utils/v4/keyvalues"

	jujucmd "github.com/juju/juju/cmd"
)

// leaderSetCommand implements the leader-set command.
type leaderSetCommand struct {
	cmd.CommandBase
	ctx      Context
	settings map[string]string
}

// NewLeaderSetCommand returns a new leaderSetCommand with the given context.
func NewLeaderSetCommand(ctx Context) (cmd.Command, error) {
	return &leaderSetCommand{ctx: ctx}, nil
}

// Info is part of the cmd.Command interface.
func (c *leaderSetCommand) Info() *cmd.Info {
	doc := `
leader-set immediate writes the supplied key/value pairs to the controller,
which will then inform non-leader units of the change. It will fail if called
without arguments, or if called by a unit that is not currently application leader.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "leader-set",
		Args:    "<key>=<value> [...]",
		Purpose: "write application leadership settings",
		Doc:     doc,
	})
}

// Init is part of the cmd.Command interface.
func (c *leaderSetCommand) Init(args []string) (err error) {
	c.settings, err = keyvalues.Parse(args, true)
	return
}

// Run is part of the cmd.Command interface.
func (c *leaderSetCommand) Run(_ *cmd.Context) error {
	err := c.ctx.WriteLeaderSettings(c.settings)
	return errors.Annotatef(err, "cannot write leadership settings")
}

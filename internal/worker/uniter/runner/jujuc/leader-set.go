// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
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

leader-set lets you distribute string key=value pairs to other units, but with the
following differences:
    there’s only one leader-settings bucket per application (not one per unit)
    only the leader can write to the bucket
    only minions are informed of changes to the bucket
    changes are propagated instantly

The instant propagation may be surprising, but it exists to satisfy the use case where
shared data can be chosen by the leader at the very beginning of the install hook.

It is strongly recommended that leader settings are always written as a self-consistent
group leader-set one=one two=two three=three.
`
	examples := `
    leader-set cluster-leader-address=10.0.0.123
`
	return jujucmd.Info(&cmd.Info{
		Name:     "leader-set",
		Args:     "<key>=<value> [...]",
		Purpose:  "Write application leadership settings.",
		Doc:      doc,
		Examples: examples,
	})
}

// Init is part of the cmd.Command interface.
func (c *leaderSetCommand) Init(args []string) (err error) {
	c.settings, err = keyvalues.Parse(args, true)
	return
}

// Run is part of the cmd.Command interface.
func (c *leaderSetCommand) Run(ctx *cmd.Context) error {
	err := c.ctx.WriteLeaderSettings(ctx, c.settings)
	return errors.Annotatef(err, "cannot write leadership settings")
}

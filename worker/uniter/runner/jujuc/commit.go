// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	jujucmd "github.com/juju/juju/cmd"
)

// CommitCommand implements the commit command.
type CommitCommand struct {
	cmd.CommandBase
	ctx CommitCommandContext
}

type CommitCommandContext interface {
	Commit() error
}

// NewCommitCommand returns a commit command.
func NewCommitCommand(ctx Context) (cmd.Command, error) {
	return &CommitCommand{ctx: ctx}, nil
}

// Info returns information about the Command.
// Info implements part of the cmd.Command interface.
func (c *CommitCommand) Info() *cmd.Info {
	doc := `
commit saves current changes made by the charm.  This is automatically done if the
hook exists successfully.  If the hook does not exist successfully, changes are
rolled back and not saved.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "commit",
		Purpose: "saves current charm data",
		Doc:     doc,
	})
}

// Run will execute the Command as directed by the options and positional
// arguments passed to Init.
// Run implements part of the cmd.Command interface.
func (c *CommitCommand) Run(ctx *cmd.Context) error {
	return c.ctx.Commit()
}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
)

const UntrackCmdName = "workload-untrack"

// NewUntrackCmd returns an UntrackCmd that uses the given hook context.
func NewUntrackCmd(ctx HookContext) (*UntrackCmd, error) {
	base, err := newCommand(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c := &UntrackCmd{
		baseCommand: base,
	}
	c.cmdInfo = cmdInfo{
		Name:    "workload-untrack",
		Summary: "stop tracking a workload",
		Doc: `
"workload-untrack" is used while a hook is running to let Juju know
that a workload has been manually stopped. The id
used to start tracking the workload must be provided.
`,
	}
	return c, nil
}

var _ cmd.Command = (*UntrackCmd)(nil)

// UntrackCmd implements the untrack command.
type UntrackCmd struct {
	*baseCommand
}

// Run runs the untrack command.
func (c *UntrackCmd) Run(ctx *cmd.Context) error {
	logger.Tracef("Running untrack command with id %q", c.ID)
	if err := c.baseCommand.Run(ctx); err != nil {
		return errors.Trace(err)
	}

	return c.compCtx.Untrack(c.ID)
}

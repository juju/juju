// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

const UntrackCmdName = "workload-untrack"

// NewUntrackCmd returns an UntrackCmd that uses the given hook context.
func NewUntrackCmd(ctx HookContext) (*UntrackCmd, error) {
	comp, err := ContextComponent(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &UntrackCmd{comp: comp}, nil
}

var _ cmd.Command = (*UntrackCmd)(nil)

// UntrackCmd implements the untrack command.
type UntrackCmd struct {
	comp Component
	id   string
}

// Init parses the command args.  Untrack requires exactly one argument - the
// name or name/id of the workload to untrack.
func (c *UntrackCmd) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("missing arg %s", idArg)
	}

	if len(args) > 1 {
		return errors.Errorf("unexpected args: %q", args[1:])
	}

	id, err := EnsureID(c.comp, args[0])
	if err != nil {
		return errors.Trace(err)
	}
	c.id = id

	return nil
}

// SetFlags implements cmd.Command.
func (*UntrackCmd) SetFlags(*gnuflag.FlagSet) {
	// noop
}

// Run runs the untrack command.
func (c *UntrackCmd) Run(*cmd.Context) error {
	logger.Tracef("Running untrack command with id %q", c.id)

	return c.comp.Untrack(c.id)
}

// Info implements cmd.Command.
func (*UntrackCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    UntrackCmdName,
		Args:    fmt.Sprintf("<%s>", idArg),
		Purpose: "stop tracking a workload",
		Doc: `
"workload-untrack" is used while a hook is running to let Juju know
that a workload has been manually stopped. The id
used to start tracking the workload must be provided.
`,
	}
}

// AllowInterspersedFlags implements cmd.Command.
func (*UntrackCmd) AllowInterspersedFlags() bool {
	return false
}

// IsSuperCommand implements cmd.Command.
func (*UntrackCmd) IsSuperCommand() bool {
	return false
}

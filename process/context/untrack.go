// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/process"
	"launchpad.net/gnuflag"
)

const UntrackCmdName = "workload-untrack"

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
	name string
}

func (c *UntrackCmd) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("missing arg %s", idArg)
	}

	if len(args) > 1 {
		return errors.Errorf("unexpected args: %q", args[1:])
	}

	c.name, _ = process.ParseID(args[0])

	if c.name == "" {
		return errors.New(idArg + " cannot be empty")
	}

	return nil
}

func (*UntrackCmd) SetFlags(*gnuflag.FlagSet) {
	// noop
}

func (c *UntrackCmd) Run(*cmd.Context) error {
	logger.Tracef("Running untrack command with name %q", c.name)

	ids, err := idsForName(c.comp, c.name)
	if err != nil {
		return errors.Trace(err)
	}
	if len(ids) == 0 {
		return errors.NotFoundf("Workload with the name %q", c.name)
	}

	err = c.comp.Untrack(ids[0])
	return err
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

func (*UntrackCmd) AllowInterspersedFlags() bool {
	return false
}

func (*UntrackCmd) IsSuperCommand() bool {
	return false
}

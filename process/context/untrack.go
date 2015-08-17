// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

const UntrackCmdName = "process-untrack"

func NewUntrackCmd(ctx HookContext) (*UntrackProcCommand, error) {
	comp, err := ContextComponent(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &UntrackProcCommand{comp: comp}, nil
}

var _ cmd.Command = (*UntrackProcCommand)(nil)

// UntrackProcCommand implements the untrack command.
type UntrackProcCommand struct {
	comp Component
	id   string
}

func (c *UntrackProcCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("missing arg %s", idArg)
	}

	if len(args) > 1 {
		return errors.Errorf("unexpected args %q", args[1:])
	}
	c.id = args[0]
	return nil
}

func (*UntrackProcCommand) SetFlags(*gnuflag.FlagSet) {
	// noop
}

func (c *UntrackProcCommand) Run(*cmd.Context) error {
	c.comp.Untrack(c.id)
	return nil
}

// Info implements cmd.Command.
func (*UntrackProcCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    UntrackCmdName,
		Args:    fmt.Sprintf("<%s>", idArg),
		Purpose: "stop tracking a workload process",
		Doc: `
"process-untrack" is used while a hook is running to let Juju know
that a workload process has been manually stopped. The id 
used to start tracking the process must be provided.
`,
	}
}

func (*UntrackProcCommand) AllowInterspersedFlags() bool {
	return false
}

func (*UntrackProcCommand) IsSuperCommand() bool {
	return false
}

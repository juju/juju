// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// TODO(ericsnow) Rename everything to "untrack" -> "unregister".

const UntrackCmdName = "payload-unregister"

// NewUntrackCmd returns a new UntrackCmd that wraps the given context.
func NewUntrackCmd(ctx HookContext) (*UntrackCmd, error) {
	compCtx, err := ContextComponent(ctx)
	if err != nil {
		// The component wasn't tracked properly.
		return nil, errors.Trace(err)
	}
	return &UntrackCmd{api: compCtx}, nil
}

// Info implements cmd.Command.
func (c UntrackCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "payload-unregister",
		Args:    "<class> <id>",
		Purpose: "stop tracking a payload",
		Doc: `
"payload-unregister" is used while a hook is running to let Juju know
that a payload has been manually stopped. The <class> and <id> provided
must match a payload that has been previously registered with juju using
payload-register.
`,
	}
}

// UntrackCmd implements the untrack command.
type UntrackCmd struct {
	*cmd.CommandBase

	api   Component
	class string
	id    string
}

// Init implements cmd.Command.
func (c *UntrackCmd) Init(args []string) error {
	if len(args) < 2 {
		return errors.Errorf("missing required arguments")
	}
	c.class = args[0]
	c.id = args[1]
	return cmd.CheckEmpty(args[2:])
}

// Run runs the untrack command.
func (c *UntrackCmd) Run(ctx *cmd.Context) error {
	//TODO(wwitzel3) make Untrack accept class and id and
	// compose the ID in the API layer using BuildID

	ID := c.class + "/" + c.id
	logger.Tracef("Running untrack command for %q", ID)

	if err := c.api.Untrack(ID); err != nil {
		return errors.Trace(err)
	}

	// We flush to state immedeiately so that status reflects the
	// workload correctly.
	if err := c.api.Flush(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

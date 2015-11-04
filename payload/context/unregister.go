// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// UnregisterCmdName is the name of the payload unregister command.
const UnregisterCmdName = "payload-unregister"

// UnregisterCmd implements the untrack command.
type UnregisterCmd struct {
	cmd.CommandBase

	hctx  Component
	class string
	id    string
}

// NewUnregisterCmd returns a new UnregisterCmd that wraps the given context.
func NewUnregisterCmd(ctx HookContext) (*UnregisterCmd, error) {
	compCtx, err := ContextComponent(ctx)
	if err != nil {
		// The component wasn't tracked properly.
		return nil, errors.Trace(err)
	}
	c := &UnregisterCmd{
		hctx: compCtx,
	}
	return c, nil
}

// Info implements cmd.Command.
func (c UnregisterCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    UnregisterCmdName,
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

// Init implements cmd.Command.
func (c *UnregisterCmd) Init(args []string) error {
	if len(args) < 2 {
		return errors.Errorf("missing required arguments")
	}

	c.class = args[0]
	c.id = args[1]

	if err := cmd.CheckEmpty(args[2:]); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Run runs the unregister command.
func (c *UnregisterCmd) Run(ctx *cmd.Context) error {
	//TODO(wwitzel3) make Unregister accept class and id and
	// compose the ID in the API layer using BuildID

	logger.Tracef(`Running unregister command with id "%s/%s"`, c.class, c.id)

	// TODO(ericsnow) Verify that Untrack gives a meaningful error when
	// the ID is not found.
	if err := c.hctx.Untrack(c.class, c.id); err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) Is the flush really necessary?

	// We flush to state immedeiately so that status reflects the
	// payload correctly.
	if err := c.hctx.Flush(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

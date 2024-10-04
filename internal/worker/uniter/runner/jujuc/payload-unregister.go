// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
)

// PayloadUnregisterCmd implements the untrack command.
type PayloadUnregisterCmd struct {
	cmd.CommandBase
	ctx ContextPayloads

	class string
	id    string
}

// NewPayloadUnregisterCmd returns a new PayloadUnregisterCmd that wraps the given context.
func NewPayloadUnregisterCmd(ctx Context) (cmd.Command, error) {
	return &PayloadUnregisterCmd{ctx: ctx}, nil
}

// Info implements cmd.Command.
func (c PayloadUnregisterCmd) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "payload-unregister",
		Args:    "<class> <id>",
		Purpose: "Stop tracking a payload.",
		Doc: `
"payload-unregister" is used while a hook is running to let Juju know
that a payload has been manually stopped. The <class> and <id> provided
must match a payload that has been previously registered with juju using
payload-register.
`,
		Examples: `
    payload-unregister monitoring 0fcgaba
`,
	})
}

// Init implements cmd.Command.
func (c *PayloadUnregisterCmd) Init(args []string) error {
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
func (c *PayloadUnregisterCmd) Run(ctx *cmd.Context) error {
	//TODO(wwitzel3) make Unregister accept class and id and
	// compose the ID in the API layer using BuildID

	logger.Tracef(`Running unregister command with id "%s/%s"`, c.class, c.id)

	// TODO(ericsnow) Verify that Untrack gives a meaningful error when
	// the ID is not found.
	if err := c.ctx.UntrackPayload(ctx, c.class, c.id); err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) Is the flush really necessary?

	// We flush to state immediately so that status reflects the
	// payload correctly.
	if err := c.ctx.FlushPayloads(ctx); err != nil {
		return errors.Trace(err)
	}

	return nil
}

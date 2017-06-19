// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
)

// PayloadUnregisterCmdName is the name of the payload unregister command.
const PayloadUnregisterCmdName = "payload-unregister"

// PayloadUnregisterCmd implements the untrack command.
type PayloadUnregisterCmd struct {
	cmd.CommandBase

	ctx   Context
	class string
	id    string
}

// NewPayloadUnregisterCmd returns a new UnregisterCmd that wraps the given context.
func NewPayloadUnregisterCmd(ctx Context) (cmd.Command, error) {
	c := &PayloadUnregisterCmd{ctx: ctx}
	return c, nil
}

// Info implements cmd.Command.
func (c PayloadUnregisterCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    PayloadUnregisterCmdName,
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
	logger.Tracef(`payload unregister command for class %v with id %v`, c.class, c.id)
	// dunno why both class and id if
	// at state level, we only need unit and class...
	// unless there is a desire to untrack using either class or id...
	p := params.UntrackPayloadParams{Class: c.class, ID: c.id}
	if err := c.ctx.UntrackPayload(p); err != nil {
		return errors.Trace(err)
	}
	return nil
}

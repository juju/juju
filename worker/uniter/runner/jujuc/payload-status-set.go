// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// PayloadStatusSetCmdName is the name of the payload status-set command.
const PayloadStatusSetCmdName = "payload-status-set"

// PayloadStatusSetCmd is a command that registers a payload with juju.
type PayloadStatusSetCmd struct {
	cmd.CommandBase

	ctx    Context
	class  string
	id     string
	status string
}

// NewPayloadStatusSetCmd returns a new StatusSetCmd that wraps the given context.
func NewPayloadStatusSetCmd(ctx Context) (cmd.Command, error) {
	c := &PayloadStatusSetCmd{ctx: ctx}
	return c, nil
}

// Info implements cmd.Command.
func (c PayloadStatusSetCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    PayloadStatusSetCmdName,
		Args:    "<class> <id> <status>",
		Purpose: "update the status of a payload",
		Doc: `
"payload-status-set" is used to update the current status of a registered payload.
The <class> and <id> provided must match a payload that has been previously
registered with juju using payload-register. The <status> must be one of the
follow: starting, started, stopping, stopped
`,
	}
}

// Init implements cmd.Command.
func (c *PayloadStatusSetCmd) Init(args []string) error {
	if len(args) < 3 {
		return errors.Errorf("missing required arguments")
	}
	c.class = args[0]
	c.id = args[1]
	c.status = args[2]
	return cmd.CheckEmpty(args[3:])
}

// Run implements cmd.Command.
func (c *PayloadStatusSetCmd) Run(ctx *cmd.Context) error {
	// dunno why both class and id  if
	// at state level, we only need unit and class...
	// unless there is a desire to untrack using either class or id...
	p := params.PayloadStatusParams{Class: c.class, ID: c.id, Status: c.status}
	if err := c.ctx.SetPayloadStatus(p); err != nil {
		return errors.Trace(err)
	}

	return nil
}

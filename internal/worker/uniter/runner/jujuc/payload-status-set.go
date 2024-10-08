// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/payloads"
)

// NewPayloadStatusSetCmd returns a new PayloadStatusSetCmd that wraps the given context.
func NewPayloadStatusSetCmd(ctx Context) (cmd.Command, error) {
	return &PayloadStatusSetCmd{ctx: ctx}, nil
}

// PayloadStatusSetCmd is a command that registers a payload with juju.
type PayloadStatusSetCmd struct {
	cmd.CommandBase
	ctx ContextPayloads

	class  string
	id     string
	status string
}

// Info implements cmd.Command.
func (c PayloadStatusSetCmd) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "payload-status-set",
		Args:    "<class> <id> <status>",
		Purpose: "Update the status of a payload.",
		Doc: `
"payload-status-set" is used to update the current status of a registered payload.
The <class> and <id> provided must match a payload that has been previously
registered with juju using payload-register. The <status> must be one of the
follow: starting, started, stopping, stopped
`,
		Examples: `
    payload-status-set monitor abcd13asa32c starting
`,
	})
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
	if err := c.validate(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := c.ctx.SetPayloadStatus(ctx, c.class, c.id, c.status); err != nil {
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

func (c *PayloadStatusSetCmd) validate(ctx *cmd.Context) error {
	return payloads.ValidateState(c.status)
}

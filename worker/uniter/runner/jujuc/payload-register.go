// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
)

// PayloadRegisterCmdName is the name of the payload register command.
const PayloadRegisterCmdName = "payload-register"

// PayloadRegisterCmd is a command that registers a payload with juju.
type PayloadRegisterCmd struct {
	cmd.CommandBase

	ctx    Context
	Type   string
	Class  string
	ID     string
	Labels []string
}

// NewPayloadRegisterCmd returns a new RegisterCmd that wraps the given context.
func NewPayloadRegisterCmd(ctx Context) (cmd.Command, error) {
	command := &PayloadRegisterCmd{ctx: ctx}
	return command, nil
}

// Info implements cmd.Command.
func (c PayloadRegisterCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    PayloadRegisterCmdName,
		Args:    "<type> <class> <id> [label...]",
		Purpose: "register a charm payload with juju",
		Doc: `
"payload-register" is used while a hook is running to let Juju know that a
payload has been started. The information used to start the payload must be
provided when "register" is run.

The payload class must correspond to one of the payloads defined in
the charm's metadata.yaml.

		`,
	}
}

// Init implements cmd.Command.
func (c *PayloadRegisterCmd) Init(args []string) error {
	if len(args) < 3 {
		return errors.Errorf("missing required arguments")
	}
	c.Type = args[0]
	c.Class = args[1]
	c.ID = args[2]
	c.Labels = args[3:]
	return nil
}

// Run implements cmd.Command.
func (c *PayloadRegisterCmd) Run(ctx *cmd.Context) error {
	p := params.TrackPayloadParams{
		Class:  c.Class,
		Type:   c.Type,
		ID:     c.ID,
		Status: payload.StateRunning,
		Labels: c.Labels,
	}
	if err := c.ctx.TrackPayload(p); err != nil {
		return errors.Trace(err)
	}
	return nil
}

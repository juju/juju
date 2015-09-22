// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
)

const endpointAddDoc = `
"endpoint-add" inserts a new endpoint for a service.
`

// EndpointAddCommand implements the endpoint-add command.
type EndpointAddCommand struct {
	cmd.CommandBase
	ctx Context

	Name      string
	Interface string
}

func NewEndpointAddCommand(ctx Context) cmd.Command {
	return &EndpointAddCommand{ctx: ctx}
}

func (c *EndpointAddCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "endpoint-add",
		Args:    "<name> <interface>",
		Purpose: "add endpoint",
		Doc:     endpointAddDoc,
	}
}

func (c *EndpointAddCommand) Init(args []string) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf("invalid arguments")
	}

	c.Name = args[0]
	c.Interface = args[1]
	return nil
}

func (c *EndpointAddCommand) Run(ctx *cmd.Context) (err error) {
	c.ctx.AddDynamicEndpoint(c.Name, c.Interface)
	return nil
}

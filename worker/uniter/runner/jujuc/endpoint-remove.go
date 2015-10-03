// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
)

const endpointRemoveDoc = `
"endpoint-remove" inserts a new endpoint for a service.
`

// EndpointRemoveCommand implements the endpoint-remove command.
type EndpointRemoveCommand struct {
	cmd.CommandBase
	ctx Context

	Name      string
	Interface string
}

func NewEndpointRemoveCommand(ctx Context) (cmd.Command, error) {
	return &EndpointRemoveCommand{ctx: ctx}, nil
}

func (c *EndpointRemoveCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "endpoint-remove",
		Args:    "<name> <interface>",
		Purpose: "remove endpoint",
		Doc:     endpointRemoveDoc,
	}
}

func (c *EndpointRemoveCommand) Init(args []string) error {
	if len(args) == 0 || len(args) > 2 {
		return fmt.Errorf("invalid arguments")
	}

	c.Name = args[0]
	c.Interface = args[1]
	return nil
}

func (c *EndpointRemoveCommand) Run(ctx *cmd.Context) (err error) {
	c.ctx.RemoveDynamicEndpoint(c.Name, c.Interface)
	return nil
}

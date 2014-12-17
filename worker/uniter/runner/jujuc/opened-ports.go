// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

// OpenedPortsCommand implements the opened-ports command.
type OpenedPortsCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output
}

func NewOpenedPortsCommand(ctx Context) cmd.Command {
	return &OpenedPortsCommand{ctx: ctx}
}

func (c *OpenedPortsCommand) Info() *cmd.Info {
	doc := `Each list entry has format <port>/<protocol> (e.g. "80/tcp") or
<from>-<to>/<protocol> (e.g. "8080-8088/udp").`
	return &cmd.Info{
		Name:    "opened-ports",
		Purpose: "lists all ports or ranges opened by the unit",
		Doc:     doc,
	}
}

func (c *OpenedPortsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *OpenedPortsCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *OpenedPortsCommand) Run(ctx *cmd.Context) error {
	unitPorts := c.ctx.OpenedPorts()
	results := make([]string, len(unitPorts))
	for i, portRange := range unitPorts {
		results[i] = portRange.String()
	}
	return c.out.Write(ctx, results)
}

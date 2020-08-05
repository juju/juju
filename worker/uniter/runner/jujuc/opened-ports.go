// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/network"
)

// OpenedPortsCommand implements the opened-ports command.
type OpenedPortsCommand struct {
	cmd.CommandBase
	ctx Context
	out cmd.Output
}

func NewOpenedPortsCommand(ctx Context) (cmd.Command, error) {
	return &OpenedPortsCommand{ctx: ctx}, nil
}

func (c *OpenedPortsCommand) Info() *cmd.Info {
	doc := `Each list entry has format <port>/<protocol> (e.g. "80/tcp") or
<from>-<to>/<protocol> (e.g. "8080-8088/udp").`
	return jujucmd.Info(&cmd.Info{
		Name:    "opened-ports",
		Purpose: "lists all ports or ranges opened by the unit",
		Doc:     doc,
	})
}

func (c *OpenedPortsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

func (c *OpenedPortsCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *OpenedPortsCommand) Run(ctx *cmd.Context) error {
	unitPortRanges := uniquePortRanges(c.ctx.OpenedPortRanges())
	results := make([]string, len(unitPortRanges))
	for i, portRange := range unitPortRanges {
		results[i] = portRange.String()
	}
	return c.out.Write(ctx, results)
}

func uniquePortRanges(portRangesByEndpoint map[string][]network.PortRange) []network.PortRange {
	var (
		res       []network.PortRange
		processed = make(map[network.PortRange]struct{})
	)

	for _, portRanges := range portRangesByEndpoint {
		for _, pr := range portRanges {
			if _, seen := processed[pr]; seen {
				continue
			}

			processed[pr] = struct{}{}
			res = append(res, pr)
		}
	}

	network.SortPortRanges(res)
	return res

}

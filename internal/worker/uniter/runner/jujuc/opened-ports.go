// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/collections/set"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/network"
)

// OpenedPortsCommand implements the opened-ports command.
type OpenedPortsCommand struct {
	cmd.CommandBase
	ctx           Context
	showEndpoints bool
	out           cmd.Output
}

func NewOpenedPortsCommand(ctx Context) (cmd.Command, error) {
	return &OpenedPortsCommand{ctx: ctx}, nil
}

func (c *OpenedPortsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "opened-ports",
		Purpose: "List all ports or port ranges opened by the unit.",
		Doc: `
opened-ports lists all ports or port ranges opened by a unit.

By default, the port range listing does not include information about the 
application endpoints that each port range applies to. Each list entry is
formatted as <port>/<protocol> (e.g. "80/tcp") or <from>-<to>/<protocol> 
(e.g. "8080-8088/udp").

If the --endpoints option is specified, each entry in the port list will be
augmented with a comma-delimited list of endpoints that the port range 
applies to (e.g. "80/tcp (endpoint1, endpoint2)"). If a port range applies to
all endpoints, this will be indicated by the presence of a '*' character
(e.g. "80/tcp (*)").

Opening ports is transactional (i.e. will take place on successfully exiting
the current hook), and therefore opened-ports will not return any values for
pending open-port operations run from within the same hook.
`,
		Examples: `
    opened-ports
`,
	})
}

func (c *OpenedPortsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
	f.BoolVar(&c.showEndpoints, "endpoints", false, "display the list of target application endpoints for each port range")
}

func (c *OpenedPortsCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *OpenedPortsCommand) Run(ctx *cmd.Context) error {
	unitPortRanges := c.ctx.OpenedPortRanges()
	if !c.showEndpoints {
		return c.renderPortsWithoutEndpointDetails(ctx, unitPortRanges)
	}

	return c.renderPortsWithEndpointDetails(ctx, unitPortRanges)
}

func (c *OpenedPortsCommand) renderPortsWithoutEndpointDetails(ctx *cmd.Context, unitPortRanges network.GroupedPortRanges) error {
	uniquePortRanges := unitPortRanges.UniquePortRanges()
	results := make([]string, len(uniquePortRanges))
	for i, portRange := range uniquePortRanges {
		results[i] = portRange.String()
	}
	return c.out.Write(ctx, results)
}

func (c *OpenedPortsCommand) renderPortsWithEndpointDetails(ctx *cmd.Context, unitPortRanges network.GroupedPortRanges) error {
	endpointsByPort := make(map[network.PortRange]set.Strings)
	for endpointName, portRanges := range unitPortRanges {
		for _, pr := range portRanges {
			if endpointsByPort[pr] == nil {
				endpointsByPort[pr] = set.NewStrings()
			}
			endpointsByPort[pr].Add(endpointName)
		}
	}

	// Sort port ranges so we can generate consistent output
	var uniquePortRanges []network.PortRange
	for pr := range endpointsByPort {
		uniquePortRanges = append(uniquePortRanges, pr)
	}
	network.SortPortRanges(uniquePortRanges)

	// Convert to port range entries and sort them by port range
	results := make([]string, len(uniquePortRanges))
	for i, pr := range uniquePortRanges {
		endpoints := endpointsByPort[pr]

		var endpointDescr string
		if endpoints.Contains("") { // all endpoints?
			endpointDescr = "*"
		} else { // sort them by name
			endpointList := endpoints.Values()
			sort.Strings(endpointList)
			endpointDescr = strings.Join(endpointList, ", ")
		}
		results[i] = fmt.Sprintf("%s (%s)", pr, endpointDescr)
	}

	return c.out.Write(ctx, results)
}

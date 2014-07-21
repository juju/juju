// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

// EndpointCommand returns the API endpoints
type EndpointCommand struct {
	envcmd.EnvCommandBase
	out     cmd.Output
	refresh bool
}

const endpointDoc = `
Returns the address of the current API server formatted as host:port.

Examples:
  $ juju api-endpoints
  10.0.3.1:17070
  $
`

func (c *EndpointCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "api-endpoints",
		Args:    "",
		Purpose: "Print the API server address",
		Doc:     endpointDoc,
	}
}

func (c *EndpointCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.refresh, "refresh", false, "connect to the API to ensure an up-to-date endpoint location")
}

// Print out the addresses of the API server endpoints.
func (c *EndpointCommand) Run(ctx *cmd.Context) error {
	apiendpoint, err := c.ConnectionEndpoint(c.refresh)
	if err != nil {
		return err
	}
	// We rely on the fact that the returned API endoint always
	// has the last address we connected to as the first address.
	address := apiendpoint.Addresses[0]
	return c.out.Write(ctx, address)
}

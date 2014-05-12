// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/juju"
)

// EndpointCommand returns the API endpoints
type EndpointCommand struct {
	envcmd.EnvCommandBase
	out     cmd.Output
	refresh bool
}

const endpointDoc = `
Returns a list of the API servers formatted as host:port
Default output format returns an api server per line.

Examples:
  $ juju api-endpoints
  10.0.3.1:17070
  $
`

func (c *EndpointCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "api-endpoints",
		Args:    "",
		Purpose: "Print the API server addresses",
		Doc:     endpointDoc,
	}
}

func (c *EndpointCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.refresh, "refresh", false, "connect to the API to ensure up-to-date endpoint locations")
}

// Print out the addresses of the API server endpoints.
func (c *EndpointCommand) Run(ctx *cmd.Context) error {
	apiendpoint, err := juju.APIEndpointForEnv(c.EnvName, c.refresh)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, apiendpoint.Addresses)
}

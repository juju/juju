// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
)

// EndpointCommand returns the API endpoints
type EndpointCommand struct {
	cmd.EnvCommandBase
	out cmd.Output
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

func (c *EndpointCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *EndpointCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

// Print out the addresses of the API server endpoints.
func (c *EndpointCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return err
	}
	environ, err := environs.NewFromName(c.EnvName, store)
	if err != nil {
		return err
	}
	_, api_info, err := environ.StateInfo()
	if err != nil {
		return err
	}
	return c.out.Write(ctx, api_info.Addrs)
}

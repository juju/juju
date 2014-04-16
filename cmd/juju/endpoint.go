// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/cmd/envcmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
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

func (c *EndpointCommand) Init(args []string) error {
	err := c.EnvCommandBase.Init()
	if err != nil {
		return err
	}
	return cmd.CheckEmpty(args)
}

func (c *EndpointCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.refresh, "refresh", false, "connect to the API to ensure up-to-date endpoint locations")
	f.IntVar(&c.NumUnits, "n", 1, "number of service units to add")
}

// Print out the addresses of the API server endpoints.
func (c *EndpointCommand) run1dot16(ctx *cmd.Context) error {
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

// Print out the addresses of the API server endpoints.
func (c *EndpointCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return err
	}
	info, err := store.ReadInfo(c.EnvName)
	addrs := info.APIEndpoint().Addresses
	if c.refresh || len(addrs) == 0 {
		// We connect to get a new list of API endpoints
		apiclient, err := juju.NewAPIClientFromName(c.EnvName)
		if err != nil {
			return fmt.Errorf(connectionError, c.EnvName, err)
		}
		apiclient.Close()
	}
	info, err := store.ReadInfo(c.EnvName)
	if addrs := info.APIEndpoint().Addresses; len(addrs) > 0 {
		return c.out.Write(ctx, addrs)
	} else {
		logger.Debugf("no cached Addresses, falling back to old method")
	}
	return c.run1dot16(ctx)
}

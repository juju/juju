// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
)

// EndpointCommand returns the API endpoints
type EndpointCommand struct {
	envcmd.EnvCommandBase
	out     cmd.Output
	refresh bool
	all     bool
}

const endpointDoc = `
Returns the address(es) of the current API server formatted as host:port.

Without arguments apt-endpoints returns the last endpoint used to successfully
connect to the API server. If a cached endpoints information is available from
the current environment's .jenv file, it is returned without trying to connect
to the API server. When no cache is available or --refresh is given, api-endpoints
connects to the API server, retrieves all known endpoints and updates the .jenv
file before returning the first one. Example:
$ juju api-endpoints
10.0.3.1:17070

If --all is given, api-endpoints returns all known endpoints. Example:
$ juju api-endpoints --all
  10.0.3.1:17070
  localhost:170170

The first endpoint is guaranteed to be an IP address and port. If a single endpoint
is available and it's a hostname, juju tries to resolve it locally first.

Additionally, you can use the --format argument to specify the output format.
Supported formats are: "yaml", "json", or "smart" (default - host:port, one per line).
`

func (c *EndpointCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "api-endpoints",
		Args:    "",
		Purpose: "print the API server address(es)",
		Doc:     endpointDoc,
	}
}

func (c *EndpointCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.refresh, "refresh", false, "connect to the API to ensure an up-to-date endpoint location")
	f.BoolVar(&c.all, "all", false, "display all known endpoints, not just the first one")
}

// Print out the addresses of the API server endpoints.
func (c *EndpointCommand) Run(ctx *cmd.Context) error {
	apiendpoint, err := endpoint(c.EnvCommandBase, c.refresh)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if errors.IsNotFound(err) || len(apiendpoint.Addresses) == 0 {
		return errors.Errorf("no API endpoints available")
	}
	if c.all {
		return c.out.Write(ctx, apiendpoint.Addresses)
	}
	return c.out.Write(ctx, apiendpoint.Addresses[0:1])
}

// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"net"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju"
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
	addresses, err := c.postprocessAddresses(apiendpoint.Addresses)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, addresses)
}

// postprocessAddresses orders and filters addresses as wanted.
func (c *EndpointCommand) postprocessAddresses(addresses []string) ([]string, error) {
	var out []string
	for _, addr := range addresses {
		epIP, err := newEndpointIP(addr)
		if err != nil {
			return nil, err
		}
		// So far we're only interested in public IPv4 addresses.
		// Other filters later depend on flags, e.g. IPv6 or private
		// addresses.
		if epIP.isPublicV4() {
			out = append(out, addr)
		}
	}
	return out, nil
}

var (
	IPv4StartPrivateA = net.IPv4(10, 0, 0, 0)
	IPv4EndPrivateA   = net.IPv4(10, 255, 255, 255)
	IPv4StartPrivateB = net.IPv4(172, 16, 0, 0)
	IPv4EndPrivateB   = net.IPv4(172, 31, 255, 255)
	IPv4StartPrivateC = net.IPv4(192, 168, 0, 0)
	IPv4EndPrivateC   = net.IPv4(192, 168, 255, 255)
)

// endpointIP is the IP address of an endpoint and provides
// convenience methods for simpler filtering.
type endpointIP struct {
	ip net.IP
}

// newEndpointIP creates an endpoint IP out of the given address.
func newEndpointIP(addr string) (*endpointIP, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &endpointIP{tcpAddr.IP}, nil
}

// isPublicV4 returns true if the address is IPv4 and public.
func (ep *endpointIP) isPublicV4() bool {
	if ep.ip.To4() == nil {
		return false
	}
	if ep.isInRange(IPv4StartPrivateA, IPv4EndPrivateA) ||
		ep.isInRange(IPv4StartPrivateB, IPv4EndPrivateB) ||
		ep.isInRange(IPv4StartPrivateC, IPv4EndPrivateC) {
		return false
	}
	return true
}

// isInRange checks if the endpoint address is in a given range.
func (ep *endpointIP) isInRange(start, end net.IP) bool {
	if bytes.Compare(ep.ip, start) >= 0 && bytes.Compare(ep.ip, end) <= 0 {
		return true
	}
	return false
}

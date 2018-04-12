// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
)

// NetworkGetCommand implements the network-get command.
type NetworkGetCommand struct {
	cmd.CommandBase
	ctx Context

	RelationId      int
	relationIdProxy gnuflag.Value

	bindingName string

	bindAddress    bool
	ingressAddress bool
	egressSubnets  bool
	keys           []string

	// deprecated
	primaryAddress bool

	out cmd.Output
}

func NewNetworkGetCommand(ctx Context) (_ cmd.Command, err error) {
	cmd := &NetworkGetCommand{ctx: ctx}
	cmd.relationIdProxy, err = NewRelationIdValue(ctx, &cmd.RelationId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return cmd, nil
}

// Info is part of the cmd.Command interface.
func (c *NetworkGetCommand) Info() *cmd.Info {
	args := "<binding-name> [--ingress-address] [--bind-address] [--egress-subnets]"
	doc := `
network-get returns the network config for a given binding name. By default
it returns the list of interfaces and associated addresses in the space for
the binding, as well as the ingress address for the binding. If defined, any
egress subnets are also returned.
If one of the following flags are specified, just that value is returned.
If more than one flag is specified, a map of values is returned.
    --bind-address: the address the local unit should listen on to serve connections, as well
                    as the address that should be advertised to its peers.
    --ingress-address: the address the local unit should advertise as being used for incoming connections.
    --egress_subnets: subnets (in CIDR notation) from which traffic on this relation will originate.
`
	return &cmd.Info{
		Name:    "network-get",
		Args:    args,
		Purpose: "get network config",
		Doc:     doc,
	}
}

// SetFlags is part of the cmd.Command interface.
func (c *NetworkGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.primaryAddress, "primary-address", false, "(deprecated) get the primary address for the binding")
	f.BoolVar(&c.bindAddress, "bind-address", false, "get the address for the binding on which the unit should listen")
	f.BoolVar(&c.ingressAddress, "ingress-address", false, "get the ingress address for the binding")
	f.BoolVar(&c.egressSubnets, "egress-subnets", false, "get the egress subnets for the binding")
	f.Var(c.relationIdProxy, "r", "specify a relation by id")
	f.Var(c.relationIdProxy, "relation", "")
}

const (
	bindAddressKey    = "bind-address"
	ingressAddressKey = "ingress-address"
	egressSubnetsKey  = "egress-subnets"
)

// Init is part of the cmd.Command interface.
func (c *NetworkGetCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no arguments specified")
	}
	c.bindingName = args[0]
	if c.bindingName == "" {
		return fmt.Errorf("no binding name specified")
	}
	if c.bindAddress {
		c.keys = append(c.keys, bindAddressKey)
	}
	if c.ingressAddress {
		c.keys = append(c.keys, ingressAddressKey)
	}
	if c.egressSubnets {
		c.keys = append(c.keys, egressSubnetsKey)
	}

	return cmd.CheckEmpty(args[1:])
}

func (c *NetworkGetCommand) Run(ctx *cmd.Context) error {
	netInfo, err := c.ctx.NetworkInfo([]string{c.bindingName}, c.RelationId)
	if err != nil {
		return errors.Trace(err)
	}

	ni, ok := netInfo[c.bindingName]
	if !ok || len(ni.Info) == 0 {
		return fmt.Errorf("no network config found for binding %q", c.bindingName)
	}
	if ni.Error != nil {
		return errors.Trace(ni.Error)
	}

	// If no specific attributes asked for,
	// print everything we know.
	if !c.primaryAddress && len(c.keys) == 0 {
		return c.out.Write(ctx, ni)
	}

	// Backwards compatibility - we just want the primary address.
	if c.primaryAddress {
		if c.ingressAddress || c.egressSubnets || c.bindAddress {
			return fmt.Errorf("--primary-address must be the only flag specified")
		}
		if len(ni.Info[0].Addresses) == 0 {
			return fmt.Errorf("no addresses attached to space for binding %q", c.bindingName)
		}
		return c.out.Write(ctx, ni.Info[0].Addresses[0].Address)
	}

	// If we want just a single value, print that, else
	// print a map of the values asked for.
	keyValues := make(map[string]interface{})
	if c.egressSubnets {
		keyValues[egressSubnetsKey] = ni.EgressSubnets
	}
	if c.ingressAddress {
		var ingressAddress string
		if len(ni.IngressAddresses) == 0 {
			if len(ni.Info[0].Addresses) == 0 {
				return fmt.Errorf("no addresses attached to space for binding %q", c.bindingName)
			}
			ingressAddress = ni.Info[0].Addresses[0].Address
		} else {
			ingressAddress = ni.IngressAddresses[0]
		}
		keyValues[ingressAddressKey] = ingressAddress
	}
	if c.bindAddress {
		keyValues[bindAddressKey] = ni.Info[0].Addresses[0].Address
	}
	if len(c.keys) == 1 {
		return c.out.Write(ctx, keyValues[c.keys[0]])
	}
	return c.out.Write(ctx, keyValues)
}

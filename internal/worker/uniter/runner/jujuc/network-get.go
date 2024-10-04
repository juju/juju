// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/rpc/params"
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
    --egress-subnets: subnets (in CIDR notation) from which traffic on this relation will originate.
`
	examples := `
    network-get dbserver
    network-get dbserver --bind-address

    See https://discourse.charmhub.io/t/charm-network-primitives/1126 for more
    in depth examples and explanation of usage.
`
	return jujucmd.Info(&cmd.Info{
		Name:     "network-get",
		Args:     args,
		Purpose:  "Get network config.",
		Doc:      doc,
		Examples: examples,
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *NetworkGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
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
	netInfo, err := c.ctx.NetworkInfo(ctx, []string{c.bindingName}, c.RelationId)
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

	// If no specific attributes were asked for, write everything we know.
	if !c.primaryAddress && len(c.keys) == 0 {
		return c.out.Write(ctx, resultToDisplay(ni))
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

	// Write the specific articles requested.
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

// These display types are used for serialising to stdout.
// We should never write raw params structs.

// interfaceAddressDisplay mirrors params.InterfaceAddress.
type interfaceAddressDisplay struct {
	Hostname string `json:"hostname" yaml:"hostname"`
	Address  string `json:"value" yaml:"value"`
	CIDR     string `json:"cidr" yaml:"cidr"`

	// This copy is used to preserve YAML serialisation that older agents
	// may be expecting. Delete them for Juju 3/4.
	AddressX string `json:"-" yaml:"address"`
}

// networkInfoDisplay mirrors params.NetworkInfo.
type networkInfoDisplay struct {
	MACAddress    string                    `json:"mac-address" yaml:"mac-address"`
	InterfaceName string                    `json:"interface-name" yaml:"interface-name"`
	Addresses     []interfaceAddressDisplay `json:"addresses" yaml:"addresses"`

	// These copies are used to preserve YAML serialisation that older agents
	// may be expecting. Delete them for Juju 3/4.
	MACAddressX    string `json:"-" yaml:"macaddress"`
	InterfaceNameX string `json:"-" yaml:"interfacename"`
}

// networkInfoResultDisplay mirrors params.NetworkInfoResult except for the
// Error member. It is assumed that we check it for nil before conversion.
type networkInfoResultDisplay struct {
	Info             []networkInfoDisplay `json:"bind-addresses,omitempty" yaml:"bind-addresses,omitempty"`
	EgressSubnets    []string             `json:"egress-subnets,omitempty" yaml:"egress-subnets,omitempty"`
	IngressAddresses []string             `json:"ingress-addresses,omitempty" yaml:"ingress-addresses,omitempty"`
}

func resultToDisplay(result params.NetworkInfoResult) networkInfoResultDisplay {
	display := networkInfoResultDisplay{
		Info:             make([]networkInfoDisplay, len(result.Info)),
		EgressSubnets:    make([]string, len(result.EgressSubnets)),
		IngressAddresses: make([]string, len(result.IngressAddresses)),
	}

	copy(display.EgressSubnets, result.EgressSubnets)
	copy(display.IngressAddresses, result.IngressAddresses)

	for i, rInfo := range result.Info {
		dInfo := networkInfoDisplay{
			MACAddress:    rInfo.MACAddress,
			InterfaceName: rInfo.InterfaceName,
			Addresses:     make([]interfaceAddressDisplay, len(rInfo.Addresses)),

			MACAddressX:    rInfo.MACAddress,
			InterfaceNameX: rInfo.InterfaceName,
		}

		for j, addr := range rInfo.Addresses {
			dInfo.Addresses[j] = interfaceAddressDisplay{
				Hostname: addr.Hostname,
				Address:  addr.Address,
				CIDR:     addr.CIDR,

				AddressX: addr.Address,
			}
		}

		display.Info[i] = dInfo
	}

	return display
}

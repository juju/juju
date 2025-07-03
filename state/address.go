// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"reflect"

	"github.com/juju/juju/core/network"
)

// address represents the location of a machine, including metadata
// about what kind of location the address describes.
//
// TODO(dimitern) Make sure we integrate this with other networking
// stuff at some point. We want to use juju-specific network names
// that point to existing documents in the networks collection.
type address struct {
	Value       string `bson:"value"`
	AddressType string `bson:"addresstype"`
	Scope       string `bson:"networkscope,omitempty"`
	Origin      string `bson:"origin,omitempty"`
	SpaceID     string `bson:"spaceid,omitempty"`
	CIDR        string `bson:"cidr,omitempty"`
}

// fromNetworkAddress is a convenience helper to create a state type
// out of the network type, here for Address with a given Origin.
func fromNetworkAddress(netAddr network.SpaceAddress, origin network.Origin) address {
	return address{
		Value:       netAddr.Value,
		AddressType: string(netAddr.Type),
		Scope:       string(netAddr.Scope),
		Origin:      string(origin),
		SpaceID:     netAddr.SpaceID.String(),
		CIDR:        netAddr.CIDR,
	}
}

// networkAddress is a convenience helper to return the state type
// as network type, here for Address.
func (addr *address) networkAddress() network.SpaceAddress {
	return network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: addr.Value,
			Type:  network.AddressType(addr.AddressType),
			Scope: network.Scope(addr.Scope),
			CIDR:  addr.CIDR,
		},
		SpaceID: network.SpaceUUID(addr.SpaceID),
	}
}

// fromNetworkAddresses is a convenience helper to create a state type
// out of the network type, here for a slice of Address with a given origin.
func fromNetworkAddresses(netAddrs network.SpaceAddresses, origin network.Origin) []address {
	addrs := make([]address, len(netAddrs))
	for i, netAddr := range netAddrs {
		addrs[i] = fromNetworkAddress(netAddr, origin)
	}
	return addrs
}

// networkAddresses is a convenience helper to return the state type
// as network type, here for a slice of Address.
func networkAddresses(addrs []address) network.SpaceAddresses {
	netAddrs := make(network.SpaceAddresses, len(addrs))
	for i, addr := range addrs {
		netAddrs[i] = addr.networkAddress()
	}
	return netAddrs
}

// addressEqual checks that two slices of network addresses are equal.
func addressesEqual(a, b []network.SpaceAddress) bool {
	return reflect.DeepEqual(a, b)
}

// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"launchpad.net/juju-core/instance"
)

// address represents the location of a machine, including metadata about what
// kind of location the address describes.
type address struct {
	Value        string
	AddressType  instance.AddressType
	NetworkName  string                `bson:",omitempty"`
	NetworkScope instance.NetworkScope `bson:",omitempty"`
}

func NewAddress(addr instance.Address) address {
	stateaddr := address{
		Value:        addr.Value,
		AddressType:  addr.Type,
		NetworkName:  addr.NetworkName,
		NetworkScope: addr.NetworkScope,
	}
	return stateaddr
}

func (addr *address) InstanceAddress() instance.Address {
	instanceaddr := instance.Address{
		Value:        addr.Value,
		Type:         addr.AddressType,
		NetworkName:  addr.NetworkName,
		NetworkScope: addr.NetworkScope,
	}
	return instanceaddr
}

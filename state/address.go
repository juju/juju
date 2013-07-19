// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"launchpad.net/juju-core/instance"
)

// address represents the location of a machine, including metadata about what
// kind of location the address describes.
type address struct {
	value        string
	addresstype  instance.AddressType
	networkname  string                `bson:",omitempty"`
	networkscope instance.NetworkScope `bson:",omitempty"`
}

func NewAddress(addr instance.Address) address {
	stateaddr := address{
		value:        addr.Value,
		addresstype:  addr.Type,
		networkname:  addr.NetworkName,
		networkscope: addr.NetworkScope,
	}
	return stateaddr
}

func (addr *address) InstanceAddress() instance.Address {
	instanceaddr := instance.Address{
		Value:        addr.value,
		Type:         addr.addresstype,
		NetworkName:  addr.networkname,
		NetworkScope: addr.networkscope,
	}
	return instanceaddr
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"launchpad.net/juju-core/names"
)

// NetworkInterface represents the state of a machine network
// interface.
type NetworkInterface struct {
	st  *State
	doc networkInterfaceDoc
}

// networkInterfaceDoc represents a network interface for a machine on
// a given network.
type networkInterfaceDoc struct {
	MACAddress string `bson:"_id"`
	// InterfaceName is the network interface name (e.g. "eth0").
	InterfaceName string
	NetworkName   string
	MachineId     string
}

func newNetworkInterface(st *State, doc *networkInterfaceDoc) *NetworkInterface {
	return &NetworkInterface{st, *doc}
}

// MACAddress returns the MAC address of the interface.
func (ni *NetworkInterface) MACAddress() string {
	return ni.doc.MACAddress
}

// InterfaceName returns the name of the interface.
func (ni *NetworkInterface) InterfaceName() string {
	return ni.doc.InterfaceName
}

// NetworkId returns the network name of the interface.
func (ni *NetworkInterface) NetworkName() string {
	return ni.doc.NetworkName
}

// NetworkTag returns the network tag of the interface.
func (ni *NetworkInterface) NetworkTag() string {
	return names.NetworkTag(ni.doc.NetworkName)
}

// MachineId returns the machine id of the interface.
func (ni *NetworkInterface) MachineId() string {
	return ni.doc.MachineId
}

// MachineTag returns the machine tag of the interface.
func (ni *NetworkInterface) MachineTag() string {
	return names.MachineTag(ni.doc.MachineId)
}

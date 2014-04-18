// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"labix.org/v2/mgo/bson"

	"launchpad.net/juju-core/names"
)

// NetworkInterface represents the state of a machine network
// interface.
type NetworkInterface struct {
	st  *State
	doc networkInterfaceDoc
}

// NetworkInterfaceInfo describes a single network interface available
// on an instance.
type NetworkInterfaceInfo struct {
	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string

	// InterfaceName is the OS-specific network device name (e.g.
	// "eth0" or "eth1.42" for a VLAN virtual interface).
	InterfaceName string

	// NetworkName is this interface's network name.
	NetworkName string

	// IsVirtual is true when the interface is a virtual device, as
	// opposed to a physical device.
	IsVirtual bool
}

// networkInterfaceDoc represents a network interface for a machine on
// a given network.
type networkInterfaceDoc struct {
	Id            bson.ObjectId `bson:"_id"`
	MACAddress    string
	InterfaceName string
	NetworkName   string
	MachineId     string
	IsVirtual     bool
}

func newNetworkInterface(st *State, doc *networkInterfaceDoc) *NetworkInterface {
	return &NetworkInterface{st, *doc}
}

func newNetworkInterfaceDoc(args NetworkInterfaceInfo) *networkInterfaceDoc {
	// This does not set the machine id.
	return &networkInterfaceDoc{
		MACAddress:    args.MACAddress,
		InterfaceName: args.InterfaceName,
		NetworkName:   args.NetworkName,
		IsVirtual:     args.IsVirtual,
	}
}

// GoString implements fmt.GoStringer.
func (ni *NetworkInterface) GoString() string {
	return fmt.Sprintf(
		"&state.NetworkInterface{machineId: %q, mac: %q, name: %q, networkName: %q, isVirtual: %v}",
		ni.MachineId(), ni.MACAddress(), ni.InterfaceName(), ni.NetworkName(), ni.IsVirtual())
}

// Id returns the internal juju-specific id of the interface.
func (ni *NetworkInterface) Id() string {
	return ni.doc.Id.String()
}

// MACAddress returns the MAC address of the interface.
func (ni *NetworkInterface) MACAddress() string {
	return ni.doc.MACAddress
}

// InterfaceName returns the name of the interface.
func (ni *NetworkInterface) InterfaceName() string {
	return ni.doc.InterfaceName
}

// NetworkName returns the network name of the interface.
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

// IsVirtual returns whether the interface represents a virtual
// device.
func (ni *NetworkInterface) IsVirtual() bool {
	return ni.doc.IsVirtual
}

// IsPhysical returns whether the interface represents a physical
// device.
func (ni *NetworkInterface) IsPhysical() bool {
	return !ni.doc.IsVirtual
}

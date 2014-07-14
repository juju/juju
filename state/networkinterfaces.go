// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
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
	// "eth0", or "eth1.42" for a VLAN virtual interface, or
	// "eth1:suffix" for a network alias).
	InterfaceName string

	// NetworkName is this interface's network name.
	NetworkName string

	// IsVirtual is true when the interface is a virtual device, as
	// opposed to a physical device (e.g. a VLAN or a network alias).
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
	IsDisabled    bool
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
		"&state.NetworkInterface{machineId: %q, mac: %q, name: %q, networkName: %q, isVirtual: %t, isDisabled: %t}",
		ni.MachineId(), ni.MACAddress(), ni.InterfaceName(), ni.NetworkName(), ni.IsVirtual(), ni.IsDisabled())
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

// RawInterfaceName return the name of the raw interface.
func (ni *NetworkInterface) RawInterfaceName() string {
	nw, err := ni.st.Network(ni.doc.NetworkName)
	if err == nil {
		return strings.TrimSuffix(ni.doc.InterfaceName, fmt.Sprintf(".%d", nw.VLANTag()))
	}
	return ni.doc.InterfaceName
}

// NetworkName returns the network name of the interface.
func (ni *NetworkInterface) NetworkName() string {
	return ni.doc.NetworkName
}

// NetworkTag returns the network tag of the interface.
func (ni *NetworkInterface) NetworkTag() string {
	return names.NewNetworkTag(ni.doc.NetworkName).String()
}

// MachineId returns the machine id of the interface.
func (ni *NetworkInterface) MachineId() string {
	return ni.doc.MachineId
}

// MachineTag returns the machine tag of the interface.
func (ni *NetworkInterface) MachineTag() string {
	return names.NewMachineTag(ni.doc.MachineId).String()
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

// IsDisabled returns whether the interface is disabled.
func (ni *NetworkInterface) IsDisabled() bool {
	return ni.doc.IsDisabled
}

// Remove removes the network interface from state.
func (ni *NetworkInterface) Remove() (err error) {
	defer errors.Maskf(&err, "cannot remove network interface %q", ni)

	ops := []txn.Op{{
		C:      ni.st.networkInterfaces.Name,
		Id:     ni.doc.Id,
		Remove: true,
	}}
	// The only abort conditions in play indicate that the network interface
	// has already been removed.
	return onAbort(ni.st.runTransaction(ops), nil)
}

// SetDisabled changes disabled state of the network interface.
func (ni *NetworkInterface) SetDisabled(isDisabled bool) (err error) {
	ops := []txn.Op{{
		C:      ni.st.networkInterfaces.Name,
		Id:     ni.doc.Id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"isdisabled", isDisabled}}}},
	}}
	err = ni.st.runTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot change disabled state on network interface: %v",
			onAbort(err, errors.NotFoundf("network interface")))
	}
	ni.doc.IsDisabled = isDisabled
	return nil
}

// Refresh refreshes the contents of the network interface from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// machine has been removed.
func (ni *NetworkInterface) Refresh() error {
	doc := networkInterfaceDoc{}
	err := ni.st.networkInterfaces.FindId(ni.doc.Id).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("network interface %#v", ni)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh network interface %q on machine %q: %v",
			ni.InterfaceName(), ni.MachineId(), err)
	}
	ni.doc = doc
	return nil
}

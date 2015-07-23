// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
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

	// Disabled returns whether the interface is disabled.
	Disabled bool
}

// networkInterfaceDoc represents a network interface for a machine on
// a given network.
type networkInterfaceDoc struct {
	Id            bson.ObjectId `bson:"_id"`
	EnvUUID       string        `bson:"env-uuid"`
	MACAddress    string        `bson:"macaddress"`
	InterfaceName string        `bson:"interfacename"`
	NetworkName   string        `bson:"networkname"`
	MachineId     string        `bson:"machineid"`
	IsVirtual     bool          `bson:"isvirtual"`
	IsDisabled    bool          `bson:"isdisabled"`
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
func (ni *NetworkInterface) NetworkTag() names.NetworkTag {
	return names.NewNetworkTag(ni.doc.NetworkName)
}

// MachineId returns the machine id of the interface.
func (ni *NetworkInterface) MachineId() string {
	return ni.doc.MachineId
}

// MachineTag returns the machine tag of the interface.
func (ni *NetworkInterface) MachineTag() names.MachineTag {
	return names.NewMachineTag(ni.doc.MachineId)
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

// Disable changes the state of the network interface to disabled. In
// case of a physical interface that has dependent virtual interfaces
// (e.g. VLANs), those will be disabled along with their parent
// interface. If the interface is already disabled, nothing happens
// and no error is returned.
func (ni *NetworkInterface) Disable() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot disable network interface %q", ni)
	return ni.setDisabled(true)
}

// Enable changes the state of the network interface to enabled. If
// the interface is already enabled, nothing happens and no error is
// returned.
func (ni *NetworkInterface) Enable() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot enable network interface %q", ni)

	return ni.setDisabled(false)
}

// Refresh refreshes the contents of the network interface from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// machine has been removed.
func (ni *NetworkInterface) Refresh() error {
	networkInterfaces, closer := ni.st.getCollection(networkInterfacesC)
	defer closer()

	doc := networkInterfaceDoc{}
	err := networkInterfaces.FindId(ni.doc.Id).One(&doc)
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

// Remove removes the network interface from state.
func (ni *NetworkInterface) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove network interface %q", ni)

	ops := []txn.Op{{
		C:      networkInterfacesC,
		Id:     ni.doc.Id,
		Remove: true,
	}}
	// The only abort conditions in play indicate that the network interface
	// has already been removed.
	return onAbort(ni.st.runTransaction(ops), nil)
}

func newNetworkInterface(st *State, doc *networkInterfaceDoc) *NetworkInterface {
	return &NetworkInterface{st, *doc}
}

func newNetworkInterfaceDoc(machineID, envUUID string, args NetworkInterfaceInfo) *networkInterfaceDoc {
	return &networkInterfaceDoc{
		Id:            bson.NewObjectId(),
		EnvUUID:       envUUID,
		MachineId:     machineID,
		MACAddress:    args.MACAddress,
		InterfaceName: args.InterfaceName,
		NetworkName:   args.NetworkName,
		IsVirtual:     args.IsVirtual,
		IsDisabled:    args.Disabled,
	}
}

// setDisabled is the internal implementation for Enable() and
// Disable().
func (ni *NetworkInterface) setDisabled(shouldDisable bool) error {
	if shouldDisable == ni.doc.IsDisabled {
		// Nothing to do.
		return nil
	}
	ops, err := ni.disableOps(shouldDisable)
	if err != nil {
		return err
	}
	ops = append(ops, assertEnvAliveOp(ni.st.EnvironUUID()))
	err = ni.st.runTransaction(ops)
	if err != nil {
		if err := checkEnvLife(ni.st); err != nil {
			return errors.Trace(err)
		}
		return onAbort(err, errors.NotFoundf("network interface"))
	}
	ni.doc.IsDisabled = shouldDisable
	return nil
}

// disableOps generates a list of transaction operations to disable or
// enable the network interface.
func (ni *NetworkInterface) disableOps(shouldDisable bool) ([]txn.Op, error) {
	ops := []txn.Op{{
		C:      networkInterfacesC,
		Id:     ni.doc.Id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"isdisabled", shouldDisable}}}},
	}}
	if shouldDisable && ni.IsPhysical() {
		// Fetch and dependent virtual interfaces on the same machine,
		// so we can disable them along with their parent.
		m, err := ni.st.Machine(ni.MachineId())
		if err != nil {
			return nil, err
		}
		ifaces, err := m.NetworkInterfaces()
		if err != nil {
			return nil, err
		}
		for _, iface := range ifaces {
			if iface.Id() == ni.Id() {
				continue
			}
			if iface.MACAddress() == ni.MACAddress() && iface.IsVirtual() {
				ops = append(ops, txn.Op{
					C:      networkInterfacesC,
					Id:     iface.doc.Id,
					Assert: txn.DocExists,
					Update: bson.D{{"$set", bson.D{{"isdisabled", shouldDisable}}}},
				})
			}
		}
	}
	return ops, nil
}

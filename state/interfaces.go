// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// NetworkInterfaceInfo describes a single network interface available on an
// instance.
type NetworkInterfaceInfo struct {
	// MACAddress is the network interface's hardware MAC address
	// (e.g. "aa:bb:cc:dd:ee:ff").
	MACAddress string

	// DeviceIndex specifies the order in which the network interface
	// appears on the host.
	DeviceIndex int

	// DeviceName is the OS-specific network device name (e.g.
	// "eth0", or "eth1.42" for a VLAN virtual interface, or
	// "eth1:suffix" for a network alias).
	DeviceName string

	// SubnetID identifies the subnet from which this interface got its
	// address(es).
	SubnetID string

	// ProviderID is the provider-specific ID for the interface (if supported,
	// otherwise empty).
	ProviderID network.Id

	// IsVirtual is true when the interface is a virtual device, as
	// opposed to a physical device (e.g. a VLAN or a network alias).
	IsVirtual bool
}

// NetworkInterface represents the state of a machine network
// interface.
type NetworkInterface struct {
	st  *State
	doc interfaceDoc
}

// interfaceDoc represents a network interface for a machine on a subnet.
type interfaceDoc struct {
	DocID       string `bson:"_id"`
	EnvUUID     string `bson:"env-uuid"`
	UUID        string `bson:"uuid"`
	ProviderID  string `bson:"providerid"`
	DeviceIndex int    `bson:"deviceindex"`
	DeviceName  string `bson:"devicename"`
	MACAddress  string `bson:"macaddress"`
	SubnetID    string `bson:"subnetid"`
	MachineID   string `bson:"machineid"`
	IsVirtual   bool   `bson:"isvirtual"`
}

// interfaceGlobalKey returns the global database key for interfaceDoc for the
// given machineID, providerID, deviceName, macAddress, and subnetID.
func interfaceGlobalKey(machineID string, providerID network.Id, deviceName, macAddress, subnetID string) string {
	return fmt.Sprintf("m#%s#p#%s#d#%s#a#%s#s#%s", machineID, providerID, deviceName, macAddress, subnetID)
}

var validDeviceName = regexp.MustCompile(`^[a-z0-9:.-]+$`)

// isValidDeviceName returns whether deviceName can be used as a valid device
// name, using the same rules MAAS applies to device names.
//
// TODO(dimitern): Move this to names perhaps?
func isValidDeviceName(deviceName string) bool {
	return validDeviceName.MatchString(deviceName)
}

// ID returns the internal juju-specific ID of the interface.
func (ni *NetworkInterface) ID() string {
	return ni.doc.DocID
}

// String implements fmt.Stringer.
func (ni *NetworkInterface) String() string {
	return fmt.Sprintf("network interface %q on machine %q", ni.doc.DeviceName, ni.doc.MachineID)
}

// UUID returns the globally unique ID of the interface.
func (ni *NetworkInterface) UUID() (utils.UUID, error) {
	return utils.UUIDFromString(ni.doc.UUID)
}

// MACAddress returns the MAC address of the interface.
func (ni *NetworkInterface) MACAddress() string {
	return ni.doc.MACAddress
}

// DeviceIndex returns the device index of the interface.
func (ni *NetworkInterface) DeviceIndex() int {
	return ni.doc.DeviceIndex
}

// DeviceName returns the name of the device representing the interface as it
// appears on the machine.
func (ni *NetworkInterface) DeviceName() string {
	return ni.doc.DeviceName
}

// SubnetID returns the ID of the subnet linked to the interface.
func (ni *NetworkInterface) SubnetID() string {
	return ni.doc.SubnetID
}

// SubnetTag returns the tag of subnet linked to the interface.
func (ni *NetworkInterface) SubnetTag() names.SubnetTag {
	return names.NewSubnetTag(ni.doc.SubnetID)
}

// ProviderId returns the provider-specific ID of the interface (if supported,
// otherwise empty).
func (ni *NetworkInterface) ProviderID() network.Id {
	return network.Id(ni.doc.ProviderID)
}

// MachineId returns the ID of this interface's machine.
func (ni *NetworkInterface) MachineID() string {
	return ni.doc.MachineID
}

// MachineTag returns the machine tag of the interface.
func (ni *NetworkInterface) MachineTag() names.MachineTag {
	return names.NewMachineTag(ni.doc.MachineID)
}

// IsVirtual returns whether the interface represents a virtual device.
func (ni *NetworkInterface) IsVirtual() bool {
	return ni.doc.IsVirtual
}

// IsPhysical returns whether the interface represents a physical device.
func (ni *NetworkInterface) IsPhysical() bool {
	return !ni.doc.IsVirtual
}

// Refresh refreshes the contents of the network interface from the underlying
// state. It returns an error that satisfies errors.IsNotFound() if the
// interface has been removed.
func (ni *NetworkInterface) Refresh() error {
	interfaces, closer := ni.st.getCollection(interfacesC)
	defer closer()

	doc := interfaceDoc{}
	err := interfaces.FindId(ni.doc.DocID).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf(ni.String())
	}
	if err != nil {
		return errors.Annotatef(err, "cannot refresh %s", ni.String())
	}
	ni.doc = doc
	return nil
}

// Remove removes the network interface from state.
func (ni *NetworkInterface) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove %s", ni.String())

	ops := []txn.Op{{
		C:      interfacesC,
		Id:     ni.doc.DocID,
		Remove: true,
	}}
	return ni.st.runTransaction(ops)
}

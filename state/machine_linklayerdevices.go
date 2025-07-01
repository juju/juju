// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/network"
)

func linkLayerDeviceDocIDFromName(st *State, machineID, deviceName string) string {
	return st.docID(linkLayerDeviceGlobalKey(machineID, deviceName))
}

// AllLinkLayerDevices returns all exiting link-layer devices of the machine.
func (m *Machine) AllLinkLayerDevices() ([]*LinkLayerDevice, error) {
	var allDevices []*LinkLayerDevice
	callbackFunc := func(resultDoc *linkLayerDeviceDoc) {
		allDevices = append(allDevices, newLinkLayerDevice(m.st, *resultDoc))
	}

	if err := m.forEachLinkLayerDeviceDoc(nil, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}
	return allDevices, nil
}

func (m *Machine) forEachLinkLayerDeviceDoc(
	docFieldsToSelect bson.D, callbackFunc func(resultDoc *linkLayerDeviceDoc),
) error {
	linkLayerDevices, closer := m.st.db().GetCollection(linkLayerDevicesC)
	defer closer()

	query := linkLayerDevices.Find(bson.D{{"machine-id", m.doc.Id}})
	if docFieldsToSelect != nil {
		query = query.Select(docFieldsToSelect)
	}
	iter := query.Iter()

	var resultDoc linkLayerDeviceDoc
	for iter.Next(&resultDoc) {
		callbackFunc(&resultDoc)
	}

	return errors.Trace(iter.Close())
}

// AllProviderInterfaceInfos returns the provider details for all of
// the link layer devices belonging to this machine. These can be used
// to identify the devices when interacting with the provider
// directly (for example, releasing container addresses).
func (m *Machine) AllProviderInterfaceInfos() ([]network.ProviderInterfaceInfo, error) {
	devices, err := m.AllLinkLayerDevices()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]network.ProviderInterfaceInfo, len(devices))
	for i, device := range devices {
		result[i].InterfaceName = device.Name()
		result[i].HardwareAddress = device.MACAddress()
		result[i].ProviderId = device.ProviderID()
	}
	return result, nil
}

// RemoveAllLinkLayerDevices removes all existing link-layer devices of the
// machine in a single transaction. No error is returned when some or all of the
// devices were already removed.
func (m *Machine) RemoveAllLinkLayerDevices() error {
	ops, err := m.removeAllLinkLayerDevicesOps()
	if err != nil {
		return errors.Trace(err)
	}
	if len(ops) == 0 {
		return nil
	}
	return m.st.db().RunTransaction(ops)
}

func (m *Machine) removeAllLinkLayerDevicesOps() ([]txn.Op, error) {
	var ops []txn.Op
	callbackFunc := func(resultDoc *linkLayerDeviceDoc) {
		removeOps := removeLinkLayerDeviceUnconditionallyOps(resultDoc.DocID)
		ops = append(ops, removeOps...)
		if resultDoc.ProviderID != "" {
			providerId := network.Id(resultDoc.ProviderID)
			op := m.st.networkEntityGlobalKeyRemoveOp("linklayerdevice", providerId)
			ops = append(ops, op)
		}
	}

	if err := m.forEachLinkLayerDeviceDoc(nil, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}

	return ops, nil
}

// LinkLayerDeviceArgs contains the arguments accepted by Machine.SetLinkLayerDevices().
type LinkLayerDeviceArgs struct {
	// Name is the name of the device as it appears on the machine.
	Name string

	// MTU is the maximum transmission unit the device can handle.
	MTU uint

	// ProviderID is a provider-specific ID of the device. Empty when not
	// supported by the provider. Cannot be cleared once set.
	ProviderID network.Id

	// Type is the type of the underlying link-layer device.
	Type network.LinkLayerDeviceType

	// MACAddress is the media access control address for the device.
	MACAddress string

	// IsAutoStart is true if the device should be activated on boot.
	IsAutoStart bool

	// IsUp is true when the device is up (enabled).
	IsUp bool

	// ParentName is the name of the parent device, which may be empty. If set,
	// it needs to be an existing device on the same machine, unless the current
	// device is inside a container, in which case ParentName can be a global
	// key of a BridgeDevice on the host machine of the container. Traffic
	// originating from a device egresses from its parent device.
	ParentName string

	// If this is device is part of a virtual switch, this field indicates
	// the type of switch (e.g. an OVS bridge ) this port belongs to.
	VirtualPortType network.VirtualPortType
}

// LinkLayerDeviceAddress contains an IP address assigned to a link-layer
// device.
type LinkLayerDeviceAddress struct {
	// DeviceName is the name of the link-layer device that has this address.
	DeviceName string

	// ConfigMethod is the method used to configure this address.
	ConfigMethod network.AddressConfigType

	// ProviderID is the provider-specific ID of the address. Empty when not
	// supported. Cannot be changed once set to non-empty.
	ProviderID network.Id

	// ProviderNetworkID is the provider-specific network ID of the address.
	// It can be left empty if not supported or known.
	ProviderNetworkID network.Id

	// ProviderSubnetID is the provider-specific subnet ID to which the
	// device is attached.
	ProviderSubnetID network.Id

	// CIDRAddress is the IP address assigned to the device, in CIDR format
	// (e.g. 10.20.30.5/24 or fc00:1234::/64).
	CIDRAddress string

	// DNSServers contains a list of DNS nameservers to use, which can be empty.
	DNSServers []string

	// DNSSearchDomains contains a list of DNS domain names to qualify
	// hostnames, and can be empty.
	DNSSearchDomains []string

	// GatewayAddress is the address of the gateway to use, which can be empty.
	GatewayAddress string

	// IsDefaultGateway is set to true if this address on this device is the
	// default gw on a machine.
	IsDefaultGateway bool

	// Origin represents the authoritative source of the address.
	// it is set using precedence, with "provider" overriding "machine".
	// It is used to determine whether the address is no longer recognised
	// and is safe to remove.
	Origin network.Origin

	// IsSecondary if true, indicates that this address is
	// not the primary address associated with the NIC.
	IsSecondary bool
}

// RemoveAllAddresses removes all assigned addresses to all devices of the
// machine, in a single transaction. No error is returned when some or all of
// the addresses were already removed.
func (m *Machine) RemoveAllAddresses() error {
	ops, err := m.removeAllAddressesOps()
	if err != nil {
		return errors.Trace(err)
	}

	return m.st.db().RunTransaction(ops)
}

func (m *Machine) removeAllAddressesOps() ([]txn.Op, error) {
	findQuery := findAddressesQuery(m.doc.Id, "")
	return m.st.removeMatchingIPAddressesDocOps(findQuery)
}

// AllDeviceAddresses returns all known addresses assigned to
// link-layer devices on the machine.
func (m *Machine) AllDeviceAddresses() ([]*Address, error) {
	var allAddresses []*Address
	callbackFunc := func(doc *ipAddressDoc) {
		allAddresses = append(allAddresses, newIPAddress(m.st, *doc))
	}

	findQuery := findAddressesQuery(m.doc.Id, "")
	if err := m.st.forEachIPAddressDoc(findQuery, callbackFunc); err != nil {
		return nil, errors.Trace(err)
	}
	return allAddresses, nil
}

// AllSpaces returns the set of spaceIDs that this machine is
// actively connected to.
// TODO(jam): 2016-12-18 This should evolve to look at the
// LinkLayerDevices directly, instead of using the Addresses
// the devices are in to link back to spaces.
func (m *Machine) AllSpaces(allSubnets network.SubnetInfos) (set.Strings, error) {
	spaces := set.NewStrings()
	callback := func(doc *ipAddressDoc) {
		// Don't bother with these. They are not in a space.
		if doc.ConfigMethod == network.ConfigLoopback || doc.SubnetCIDR == "" {
			return
		}

		for _, sub := range allSubnets {
			if sub.CIDR == doc.SubnetCIDR {
				spaces.Add(sub.SpaceID.String())
				break
			}
		}
	}
	if err := m.st.forEachIPAddressDoc(findAddressesQuery(m.doc.Id, ""), callback); err != nil {
		return nil, errors.Trace(err)
	}

	return spaces, nil
}

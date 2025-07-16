// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/network"
)

// ipAddressDoc describes the persistent state of an IP address assigned to a
// link-layer network device (a.k.a. network interface card - NIC).
type ipAddressDoc struct {
	// DocID is the IP address global key, prefixed by ModelUUID.
	DocID string `bson:"_id"`

	// ModelUUID is the UUID of the model this IP address belongs to.
	ModelUUID string `bson:"model-uuid"`

	// ProviderID is a provider-specific ID of the IP address, prefixed by
	// ModelUUID. Empty when not supported by the provider.
	ProviderID string `bson:"providerid,omitempty"`

	// ProviderNetworkID is a provider-specific ID for this address's network.
	// Empty when not supported by the provider.
	ProviderNetworkID string `bson:"provider-network-id,omitempty"`

	// ProviderSubnetID is a provider-specific ID for this address's subnet.
	// Empty when not supported by the provider.
	ProviderSubnetID string `bson:"provider-subnet-id,omitempty"`

	// DeviceName is the name of the link-layer device this IP address is
	// assigned to.
	DeviceName string `bson:"device-name"`

	// MachineID is the ID of the machine this IP address's device belongs to.
	MachineID string `bson:"machine-id"`

	// SubnetCIDR is the CIDR of the subnet this IP address belongs to.
	// The CIDR will either match a known provider subnet or a machine-local
	// subnet (like 10.0.3.0/24 or 127.0.0.0/8).
	SubnetCIDR string `bson:"subnet-cidr"`

	// ConfigMethod is the method used to configure this IP address.
	ConfigMethod network.AddressConfigType `bson:"config-method"`

	// Value is the value of the configured IP address, e.g. 192.168.1.2 or
	// 2001:db8::/64.
	Value string `bson:"value"`

	// DNSServers contains a list of DNS nameservers that apply to this IP
	// address's device. Can be empty.
	DNSServers []string `bson:"dns-servers,omitempty"`

	// DNSSearchDomains contains a list of DNS domain names used to qualify
	// hostnames, and can be empty.
	DNSSearchDomains []string `bson:"dns-search-domains,omitempty"`

	// GatewayAddress is the IP address of the gateway this IP address's device
	// uses. Can be empty.
	GatewayAddress string `bson:"gateway-address,omitempty"`

	// IsDefaultGateway is set to true if that device/subnet is the default
	// gateway for the machine.
	IsDefaultGateway bool `bson:"is-default-gateway,omitempty"`

	// Origin represents the authoritative source of the ipAddress.
	// It is expected that either the provider gave us this address or the
	// machine gave us this address.
	// Giving us this information allows us to reason about when a ipAddress is
	// in use.
	// This should always be required, hence the lack of omitempty (upgrade
	// steps should correctly assign this for all addresses)
	Origin network.Origin `bson:"origin"`

	// IsShadow indicates whether this address is virtual/floating/shadow
	// address assigned to a NIC by a provider rather than being associated
	// directly with a device on-machine.
	IsShadow bool `bson:"is-shadow,omitempty"`

	// IsSecondary if true, indicates that this address is not the primary
	// address associated with the NIC.
	// Such addresses can be added by clustering solutions like Pacemaker.
	// We need to prevent these addresses being supplied with higher
	// priority than primary addresses in returns to network-get calls.
	IsSecondary bool `bson:"is-secondary,omitempty"`
}

// Address represents the state of an IP address assigned to a link-layer
// network device on a machine.
type Address struct {
	st  *State
	doc ipAddressDoc
}

func newIPAddress(st *State, doc ipAddressDoc) *Address {
	return &Address{st: st, doc: doc}
}

// DocID returns the globally unique ID of the IP address, including the model
// UUID as prefix.
func (addr *Address) DocID() string {
	return addr.st.docID(addr.doc.DocID)
}

// ProviderID returns the provider-specific IP address ID, if set.
func (addr *Address) ProviderID() network.Id {
	return network.Id(addr.doc.ProviderID)
}

// ProviderSubnetID returns the provider-specific subnet ID, if set.
func (addr *Address) ProviderSubnetID() network.Id {
	return network.Id(addr.doc.ProviderSubnetID)
}

// ProviderNetworkID returns the provider-specific network ID, if set.
func (addr *Address) ProviderNetworkID() network.Id {
	return network.Id(addr.doc.ProviderNetworkID)
}

// MachineID returns the ID of the machine this IP address belongs to.
func (addr *Address) MachineID() string {
	return addr.doc.MachineID
}

// Machine returns the Machine this IP address belongs to.
func (addr *Address) Machine() (*Machine, error) {
	return addr.st.Machine(addr.doc.MachineID)
}

// DeviceName returns the name of the link-layer device this IP address is
// assigned to.
func (addr *Address) DeviceName() string {
	return addr.doc.DeviceName
}

// Device returns the LinkLayerDevice this IP address is assigned to.
func (addr *Address) Device() (*LinkLayerDevice, error) {
	devID := linkLayerDeviceDocIDFromName(addr.st, addr.doc.MachineID, addr.doc.DeviceName)
	dev, err := addr.st.LinkLayerDevice(devID)
	return dev, errors.Trace(err)
}

func linkLayerDeviceDocIDFromName(st *State, machineID, deviceName string) string {
	return st.docID(linkLayerDeviceGlobalKey(machineID, deviceName))
}

// SubnetCIDR returns the CIDR of the subnet this IP address comes from.
func (addr *Address) SubnetCIDR() string {
	return addr.doc.SubnetCIDR
}

// ConfigMethod returns the AddressConfigMethod used for this IP address.
func (addr *Address) ConfigMethod() network.AddressConfigType {
	return addr.doc.ConfigMethod
}

// LoopbackConfigMethod returns whether AddressConfigMethod used for this IP
// address was loopback.
func (addr *Address) LoopbackConfigMethod() bool {
	return addr.doc.ConfigMethod == network.ConfigLoopback
}

// Value returns the value of this IP address.
func (addr *Address) Value() string {
	return addr.doc.Value
}

// DNSServers returns the list of DNS nameservers to use, which can be empty.
func (addr *Address) DNSServers() []string {
	return addr.doc.DNSServers
}

// DNSSearchDomains returns the list of DNS domains to use for qualifying
// hostnames. Can be empty.
func (addr *Address) DNSSearchDomains() []string {
	return addr.doc.DNSSearchDomains
}

// GatewayAddress returns the gateway address to use, which can be empty.
func (addr *Address) GatewayAddress() string {
	return addr.doc.GatewayAddress
}

// IsDefaultGateway returns true if this address is used for the default
// gateway on the machine.
func (addr *Address) IsDefaultGateway() bool {
	return addr.doc.IsDefaultGateway
}

// Origin represents the authoritative source of the ipAddress.
// it is set using precedence, with "provider" overriding "machine".
// It is used to determine whether the address is no longer recognised
// and is safe to remove.
func (addr *Address) Origin() network.Origin {
	return addr.doc.Origin
}

// IsShadow indicates whether this address is virtual/floating/shadow
// address. In cross-model relations, we may want to return this address
// for a device if its non-shadow address is bound to a cloud-local
// subnet.
func (addr *Address) IsShadow() bool {
	return addr.doc.IsShadow
}

// IsSecondary if true, indicates that this address is not the primary
// address associated with the NIC.
func (addr *Address) IsSecondary() bool {
	return addr.doc.IsSecondary
}

// String returns a human-readable representation of the IP address.
func (addr *Address) String() string {
	return fmt.Sprintf(
		"%s address %q of device %q on machine %q",
		addr.doc.ConfigMethod, addr.doc.Value,
		addr.doc.DeviceName, addr.doc.MachineID,
	)
}

// RemoveOps returns transaction operations that will ensure that the
// address is not present in the collection and that if set,
// its provider ID is removed from the global register.
func (addr *Address) RemoveOps() []txn.Op {
	ops := []txn.Op{{
		C:      ipAddressesC,
		Id:     addr.doc.DocID,
		Remove: true,
	}}

	if addr.ProviderID() != "" {
		ops = append(ops, addr.st.networkEntityGlobalKeyRemoveOp("address", addr.ProviderID()))
	}

	return ops
}

func findAddressesQuery(machineID, deviceName string) bson.D {
	var query bson.D
	if machineID != "" {
		query = append(query, bson.DocElem{Name: "machine-id", Value: machineID})
	}
	if deviceName != "" {
		query = append(query, bson.DocElem{Name: "device-name", Value: deviceName})
	}
	return query
}

func (st *State) removeMatchingIPAddressesDocOps(findQuery bson.D) ([]txn.Op, error) {
	var ops []txn.Op
	callbackFunc := func(resultDoc *ipAddressDoc) {
		addr := &Address{st: st, doc: *resultDoc}
		ops = append(ops, addr.RemoveOps()...)
	}

	err := st.forEachIPAddressDoc(findQuery, callbackFunc)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return ops, nil
}

func (st *State) forEachIPAddressDoc(findQuery bson.D, callbackFunc func(resultDoc *ipAddressDoc)) error {
	addresses, closer := st.db().GetCollection(ipAddressesC)
	defer closer()

	query := addresses.Find(findQuery)
	iter := query.Iter()

	var resultDoc ipAddressDoc
	for iter.Next(&resultDoc) {
		callbackFunc(&resultDoc)
	}

	return errors.Trace(iter.Close())
}

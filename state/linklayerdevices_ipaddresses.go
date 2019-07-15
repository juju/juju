// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// ipAddressDoc describes the persistent state of an IP address assigned to a
// link-layer network device (a.k.a network interface card - NIC).
type ipAddressDoc struct {
	// DocID is the IP address global key, prefixed by ModelUUID.
	DocID string `bson:"_id"`

	// ModelUUID is the UUID of the model this IP address belongs to.
	ModelUUID string `bson:"model-uuid"`

	// ProviderID is a provider-specific ID of the IP address, prefixed by
	// ModelUUID. Empty when not supported by the provider.
	ProviderID string `bson:"providerid,omitempty"`

	// DeviceName is the name of the link-layer device this IP address is
	// assigned to.
	DeviceName string `bson:"device-name"`

	// MachineID is the ID of the machine this IP address's device belongs to.
	MachineID string `bson:"machine-id"`

	// SubnetCIDR is the CIDR of the subnet this IP address belongs to. The CIDR
	// will either match a known provider subnet or a machine-local subnet (like
	// 10.0.3.0/24 or 127.0.0.0/8).
	SubnetCIDR string `bson:"subnet-cidr"`

	// ConfigMethod is the method used to configure this IP address.
	ConfigMethod AddressConfigMethod `bson:"config-method"`

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

	// IsDefaultGateway is set to true if that device/subnet is the default gw for the machine
	IsDefaultGateway bool `bson:"is-default-gateway,omitempty"`
}

// AddressConfigMethod is the method used to configure a link-layer device's IP
// address.
type AddressConfigMethod string

const (
	// LoopbackAddress is used for IP addresses of LoopbackDevice types.
	LoopbackAddress AddressConfigMethod = "loopback"

	// StaticAddress is used for statically configured addresses.
	StaticAddress AddressConfigMethod = "static"

	// DynamicAddress is used for addresses dynamically configured via DHCP.
	DynamicAddress AddressConfigMethod = "dynamic"

	// ManualAddress is used for manually configured addresses.
	ManualAddress AddressConfigMethod = "manual"
)

// IsValidAddressConfigMethod returns whether the given value is a valid method
// to configure a link-layer network device's IP address.
func IsValidAddressConfigMethod(value string) bool {
	switch AddressConfigMethod(value) {
	case LoopbackAddress, StaticAddress, DynamicAddress, ManualAddress:
		return true
	}
	return false
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

// NetworkAddress returns the address transformed to a network.Address.
func (addr *Address) NetworkAddress() network.Address {
	return network.NewAddress(addr.Value())
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

// MachineID returns the ID of the machine this IP address belongs to.
func (addr *Address) MachineID() string {
	return addr.doc.MachineID
}

// Machine returns the Machine this IP address belongs to.
func (addr *Address) Machine() (*Machine, error) {
	return addr.st.Machine(addr.doc.MachineID)
}

// machineProxy is a convenience wrapper for calling Machine methods from an
// *Address.
func (addr *Address) machineProxy() *Machine {
	return &Machine{st: addr.st, doc: machineDoc{Id: addr.doc.MachineID}}
}

// DeviceName returns the name of the link-layer device this IP address is
// assigned to.
func (addr *Address) DeviceName() string {
	return addr.doc.DeviceName
}

// Device returns the LinkLayerDevice this IP address is assigned to.
func (addr *Address) Device() (*LinkLayerDevice, error) {
	return addr.machineProxy().LinkLayerDevice(addr.doc.DeviceName)
}

// SubnetCIDR returns the CIDR of the subnet this IP address comes from.
func (addr *Address) SubnetCIDR() string {
	return addr.doc.SubnetCIDR
}

// Subnet returns the Subnet this IP address comes from. Returns nil and
// errors.NotFoundError if the address comes from an unknown subnet (i.e.
// machine-local one).
func (addr *Address) Subnet() (*Subnet, error) {
	return addr.st.Subnet(addr.doc.SubnetCIDR)
}

// ConfigMethod returns the AddressConfigMethod used for this IP address.
func (addr *Address) ConfigMethod() AddressConfigMethod {
	return addr.doc.ConfigMethod
}

// LoopbackConfigMethod returns whether AddressConfigMethod used for this IP
// address was loopback.
func (addr *Address) LoopbackConfigMethod() bool {
	return addr.doc.ConfigMethod == LoopbackAddress
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

// IsDefaultGateway returns true if this address is used for the default gw on the machine.
func (addr *Address) IsDefaultGateway() bool {
	return addr.doc.IsDefaultGateway
}

// String returns a human-readable representation of the IP address.
func (addr *Address) String() string {
	return fmt.Sprintf(
		"%s address %q of device %q on machine %q",
		addr.doc.ConfigMethod, addr.doc.Value,
		addr.doc.DeviceName, addr.doc.MachineID,
	)
}

func (addr *Address) globalKey() string {
	return ipAddressGlobalKey(addr.doc.MachineID, addr.doc.DeviceName, addr.doc.Value)
}

func ipAddressGlobalKey(machineID, deviceName, address string) string {
	deviceGlobalKey := linkLayerDeviceGlobalKey(machineID, deviceName)
	if deviceGlobalKey == "" || address == "" {
		return ""
	}
	return deviceGlobalKey + "#ip#" + address
}

// Remove removes the IP address, if it exists. No error is returned when the
// address was already removed.
func (addr *Address) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove %s", addr)

	removeOp := removeIPAddressDocOp(addr.doc.DocID)
	ops := []txn.Op{removeOp}
	if addr.ProviderID() != "" {
		op := addr.st.networkEntityGlobalKeyRemoveOp("address", addr.ProviderID())
		ops = append(ops, op)
	}
	return addr.st.db().RunTransaction(ops)
}

// removeIPAddressDocOpOp returns an operation to remove the ipAddressDoc
// matching the given ipAddressDocID, without asserting it still exists.
func removeIPAddressDocOp(ipAddressDocID string) txn.Op {
	return txn.Op{
		C:      ipAddressesC,
		Id:     ipAddressDocID,
		Remove: true,
	}
}

// insertIPAddressDocOp returns an operation inserting the given newDoc,
// asserting it does not exist yet.
func insertIPAddressDocOp(newDoc *ipAddressDoc) txn.Op {
	return txn.Op{
		C:      ipAddressesC,
		Id:     newDoc.DocID,
		Assert: txn.DocMissing,
		Insert: *newDoc,
	}
}

func strsDiffer(a, b []string) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

// updateIPAddressDocOp returns an operation updating the fields of existingDoc
// with the respective values of those fields in newDoc. DocID, ModelUUID,
// Value, MachineID, and DeviceName cannot be changed. ProviderID cannot be
// changed once set. DNSServers and DNSSearchDomains are deleted when nil. In
// all other cases newDoc values overwrites existingDoc values.
func updateIPAddressDocOp(existingDoc, newDoc *ipAddressDoc) (txn.Op, bool) {
	changes := make(bson.M)
	deletes := make(bson.M)
	if existingDoc.ProviderID == "" && newDoc.ProviderID != "" {
		// Only allow changing the ProviderID if it was empty.
		changes["providerid"] = newDoc.ProviderID
	}
	if existingDoc.ConfigMethod != newDoc.ConfigMethod {
		changes["config-method"] = newDoc.ConfigMethod
	}

	if existingDoc.SubnetCIDR != newDoc.SubnetCIDR {
		changes["subnet-cidr"] = newDoc.SubnetCIDR
	}

	if strsDiffer(newDoc.DNSServers, existingDoc.DNSServers) {
		if len(newDoc.DNSServers) == 0 {
			deletes["dns-servers"] = 1
		} else {
			changes["dns-servers"] = newDoc.DNSServers
		}
	}
	if strsDiffer(newDoc.DNSSearchDomains, existingDoc.DNSSearchDomains) {
		if len(newDoc.DNSSearchDomains) == 0 {
			deletes["dns-search-domains"] = 1
		} else {
			changes["dns-search-domains"] = newDoc.DNSSearchDomains
		}
	}

	if existingDoc.GatewayAddress != newDoc.GatewayAddress {
		changes["gateway-address"] = newDoc.GatewayAddress
	}

	var updates bson.D
	if len(changes) > 0 {
		updates = append(updates, bson.DocElem{Name: "$set", Value: changes})
	}
	if len(deletes) > 0 {
		updates = append(updates, bson.DocElem{Name: "$unset", Value: deletes})
	}

	return txn.Op{
		C:      ipAddressesC,
		Id:     existingDoc.DocID,
		Assert: txn.DocExists,
		Update: updates,
	}, len(updates) > 0
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
		ops = append(ops, removeIPAddressDocOp(resultDoc.DocID))
		if resultDoc.ProviderID != "" {
			addrID := network.Id(resultDoc.ProviderID)
			op := st.networkEntityGlobalKeyRemoveOp("address", addrID)
			ops = append(ops, op)
		}
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

// AllIPAddresses returns all ip addresses in the model.
func (st *State) AllIPAddresses() (addresses []*Address, err error) {
	addressesCollection, closer := st.db().GetCollection(ipAddressesC)
	defer closer()

	sdocs := []ipAddressDoc{}
	err = addressesCollection.Find(bson.D{}).All(&sdocs)
	if err != nil {
		return nil, errors.Errorf("cannot get all ip addresses")
	}
	for _, a := range sdocs {
		addresses = append(addresses, newIPAddress(st, a))
	}
	return addresses, nil
}

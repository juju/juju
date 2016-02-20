// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// interfaceDoc describes the persistent state of a machine network interface.
type interfaceDoc struct {
	// DocID is the interface global key, prefixed by ModelUUID.
	DocID string `bson:"_id"`

	// Name is the device name of the interface as it appears on the machine.
	Name string `bson:"name"`

	// ModelUUID is the UUID of the model this interface is part of.
	ModelUUID string `bson:"model-uuid"`

	// Index is the zero-based device index of the interface as it appears on
	// the machine.
	Index uint `bson:"index"`

	// MTU is the maximum transmission unit the interface can handle.
	MTU uint `bson:"mtu"`

	// ProviderID is a provider-specific ID of the interface, prefixed by
	// ModelUUID. Empty when not supported by the provider.
	ProviderID string `bson:"providerid,omitempty"`

	// MachineID is the ID of the machine where this interface is located.
	MachineID string `bson:"machine-id"`

	// Type is the type of the interface related to the underlying device.
	Type InterfaceType `bson:"type"`

	// HardwareAddress is the hardware address for the interface, usually a MAC
	// address.
	HardwareAddress string `bson:"hardware-address"`

	// IsAutoStart is true if the interface should be activated on boot.
	IsAutoStart bool `bson:"is-auto-start"`

	// IsUp is true when the interface is up (enabled).
	IsUp bool `bson:"is-up"`

	// ParentName is the name of the parent interface, which may be empty.
	ParentName string `bson:"parent-name"`

	// DNSServers is an optional list of DNS nameservers that apply to this
	// interface.
	DNSServers []string `bson:"dns-servers,omitempty"`

	// DNSSearchDomains is an optional list of DNS domain names used to
	// construct a fully-qualified hostname.
	DNSSearchDomains []string `bson:"dns-search-domains,omitempty"`

	// GatewayAddress is the gateway to use for this interface, which may be
	// empty.
	GatewayAddress string `bson:"gateway-address,omitempty"`
}

// InterfaceType defines the type of a machine network interface.
type InterfaceType string

const (
	// UnknownInterface is used for interfaces with unknown type.
	UnknownInterface InterfaceType = "unknown"

	// LoopbackInterface is used for loopback interfaces.
	LoopbackInterface InterfaceType = "loopback"

	// EthernetInterface is used for interfaces representing Ethernet (IEEE
	// 802.3) devices.
	EthernetInterface InterfaceType = "ethernet"

	// VLAN_8021QInterface is used for interfaces representing IEEE 802.1Q VLAN
	// devices.
	VLAN_8021QInterface InterfaceType = "802.1q"

	// BondInterface is used for interfaces representing bonding devices.
	BondInterface InterfaceType = "bond"

	// BridgeInterface is used for interfaces representing an OSI layer-2 bridge
	// device.
	BridgeInterface InterfaceType = "bridge"
)

// IsValidInterfaceType returns whether the given value is a valid interface
// type.
func IsValidInterfaceType(value string) bool {
	switch InterfaceType(value) {
	case UnknownInterface, LoopbackInterface, EthernetInterface,
		VLAN_8021QInterface, BondInterface, BridgeInterface:
		return true
	}
	return false
}

// Interface represents the state of a machine network interface.
type Interface struct {
	st  *State
	doc interfaceDoc
}

func newInterface(st *State, doc interfaceDoc) *Interface {
	return &Interface{st: st, doc: doc}
}

// DocID returns the globally unique interface ID, including the model UUID as
// prefix.
func (nic *Interface) DocID() string {
	return nic.st.docID(nic.doc.DocID)
}

// Name returns the interface name as it appears on its machine.
func (nic *Interface) Name() string {
	return nic.doc.Name
}

// Index returns the interface's device index as it appears on its machine.
func (nic *Interface) Index() uint {
	return nic.doc.Index
}

// MTU returns the maximum transmission unit the interface can handle.
func (nic *Interface) MTU() uint {
	return nic.doc.MTU
}

// ProviderID returns the provider-specific interface ID, if set.
func (nic *Interface) ProviderID() network.Id {
	return network.Id(nic.localProviderID())
}

func (nic *Interface) localProviderID() string {
	return nic.st.localID(nic.doc.ProviderID)
}

// MachineID returns the ID of the machine this interface is on.
func (nic *Interface) MachineID() string {
	return nic.doc.MachineID
}

// Machine returns the Machine of this interface.
func (nic *Interface) Machine() (*Machine, error) {
	return nic.st.Machine(nic.doc.MachineID)
}

// Type returns the type of the interface.
func (nic *Interface) Type() InterfaceType {
	return nic.doc.Type
}

// HardwareAddress returns the hardware address of the interface, usually a MAC
// address.
func (nic *Interface) HardwareAddress() string {
	return nic.doc.HardwareAddress
}

// IsAutoStart returns whether the interface is configured to automatically
// start on boot.
func (nic *Interface) IsAutoStart() bool {
	return nic.doc.IsAutoStart
}

// IsUp returns whether the interface is currently up.
func (nic *Interface) IsUp() bool {
	return nic.doc.IsUp
}

// ParentName returns the name of this interface's parent interface, if set.
func (nic *Interface) ParentName() string {
	return nic.doc.ParentName
}

// ParentInterface returns the Interface corresponding to this interface's
// parent (if specified). When no parent interface name is set, it returns nil
// and no error.
func (nic *Interface) ParentInterface() (*Interface, error) {
	if nic.doc.ParentName == "" {
		return nil, nil
	}

	return nic.machineProxy().Interface(nic.doc.ParentName)
}

// machineProxy is a convenience wrapper for calling Machine.Interface() or
// Machine.forEachInterfaceDoc() from *Interface.
func (nic *Interface) machineProxy() *Machine {
	return &Machine{st: nic.st, doc: machineDoc{Id: nic.doc.MachineID}}
}

// Refresh refreshes the contents of the inteface from the underlying state. It
// returns an error that satisfies errors.IsNotFound if the interface has been
// removed.
func (nic *Interface) Refresh() error {
	freshCopy, err := nic.machineProxy().Interface(nic.doc.Name)
	if errors.IsNotFound(err) {
		return err
	} else if err != nil {
		return errors.Annotatef(err, "cannot refresh interface %s", nic)
	}
	nic.doc = freshCopy.doc
	return nil
}

// Remove deletes the interface, if it exists. No error is returned when the
// interface was already removed. Error is returned if this interface is a
// parent to one or more existing interfaces.
func (nic *Interface) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove %s", nic)

	childrenNames := set.NewStrings()
	collectChildren := func(resultDoc *interfaceDoc) {
		if resultDoc.ParentName == nic.doc.Name {
			childrenNames.Add(resultDoc.Name)
		}
	}
	selectOnly := bson.D{{"_id", 1}, {"name", 1}, {"parent-name", 1}}
	err = nic.machineProxy().forEachInterfaceDoc(selectOnly, collectChildren)
	if err != nil {
		return errors.Trace(err)
	}
	if !childrenNames.IsEmpty() {
		names := strings.Join(childrenNames.SortedValues(), ", ")
		return errors.Errorf("parent interface to: %s", names)
	}

	ops := []txn.Op{removeInterfaceOp(nic.doc.DocID)}
	return nic.st.runTransaction(ops)
}

// insertInterfaceDocOp returns an operation inserting the given
// newInterfaceDoc, asserting it does not exist yet.
func insertInterfaceDocOp(newInterfaceDoc *interfaceDoc) txn.Op {
	return txn.Op{
		C:      interfacesC,
		Id:     newInterfaceDoc.DocID,
		Assert: txn.DocMissing,
		Insert: *newInterfaceDoc,
	}
}

// removeInterfaceOp returns the operation needed to remove the interface with
// the given interfaceDocID.
func removeInterfaceOp(interfaceDocID string) txn.Op {
	return txn.Op{
		C:      interfacesC,
		Id:     interfaceDocID,
		Remove: true,
	}
}

// assertInterfaceExistsOp returns an operation asserting the interface matching
// interfaceDocID exists.
func assertInterfaceExistsOp(interfaceDocID string) txn.Op {
	return txn.Op{
		C:      interfacesC,
		Id:     interfaceDocID,
		Assert: txn.DocExists,
	}
}

// DNSServers returns the list of DNS nameservers that apply to this interface,
// if they are known, or an empty slice otherwise.
func (nic *Interface) DNSServers() []string {
	return nic.doc.DNSServers
}

// DNSSearchDomains returns the list of DNS domain names used to qualify
// hostnames. Can be empty when not set.
func (nic *Interface) DNSSearchDomains() []string {
	return nic.doc.DNSSearchDomains
}

// GatewayAddress returns the address of the gateway to use for this interface,
// if set.
func (nic *Interface) GatewayAddress() string {
	return nic.doc.GatewayAddress
}

// String returns the interface as a human-readable string.
func (nic *Interface) String() string {
	return fmt.Sprintf("%s interface %q on machine %q", nic.doc.Type, nic.doc.Name, nic.doc.MachineID)
}

func (nic *Interface) globalKey() string {
	return interfaceGlobalKey(nic.doc.MachineID, nic.doc.Name)
}

func interfaceGlobalKey(machineID, interfaceName string) string {
	if machineID == "" || interfaceName == "" {
		return ""
	}
	return "m#" + machineID + "i#" + interfaceName
}

// IsValidInterfaceName returns whether the given interfaceName is a valid
// network device name, depending on the runtime.GOOS value.
func IsValidInterfaceName(interfaceName string) bool {
	if runtimeGOOS == "linux" {
		return isValidLinuxDeviceName(interfaceName)
	}
	hasHash := strings.Contains(interfaceName, "#")
	return !hasHash && stringLengthBetween(interfaceName, 1, 255)
}

// runtimeGOOS is defined to allow patching in tests.
var runtimeGOOS = runtime.GOOS

// isValidLinuxDeviceName returns whether the given deviceName is valid,
// using the same criteria as dev_valid_name(9) in the Linux kernel:
// - no whitespace allowed
// - length from 1 to 15 ASCII characters
// - literal "." and ".." as names are not allowed.
// Additionally, we don't allow "#" in the name.
func isValidLinuxDeviceName(deviceName string) bool {
	hasWhitespace := whitespaceReplacer.Replace(deviceName) != deviceName
	isDot, isDoubleDot := deviceName == ".", deviceName == ".."
	hasValidLength := stringLengthBetween(deviceName, 1, 15)
	hasHash := strings.Contains(deviceName, "#")

	return hasValidLength && !(hasHash || hasWhitespace || isDot || isDoubleDot)
}

// whitespaceReplacer strips whitespace characters from the input string.
var whitespaceReplacer = strings.NewReplacer(
	" ", "",
	"\t", "",
	"\v", "",
	"\n", "",
	"\r", "",
)

func stringLengthBetween(value string, minLength, maxLength uint) bool {
	length := uint(len(value))
	return length >= minLength && length <= maxLength
}

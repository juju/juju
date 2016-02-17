// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"

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

	// IsActive is true when the interface is active (enabled).
	IsActive bool `bson:"is-active"`

	// ParentName is the name of the parent interface or empty.
	ParentName string `bson:"parent-name"`

	// DNSServers is an optional list of DNS nameservers that apply for this
	// interface.
	DNSServers []string `bson:"dns-servers,omitempty"`

	// DNSDomain is an optional default DNS domain name to use for this
	// interface.
	DNSDomain string `bson:"dns-domain,omitempty"`

	// GatewayAddress is the optional gateway to use for this interface.
	GatewayAddress string `bson:"gateway-address,omitempty"`
}

// InterfaceType defines the type of a machine network interface.
type InterfaceType string

const (
	// UnknownInterface is used for interfaces with unknown type.
	UnknownInterface InterfaceType = "unknown"

	// LoopbackInterface is used for loopback interfaces.
	LoopbackInterface InterfaceType = "loopback"

	// EthernetInterface is used for interfaces representing Ethernet devices.
	EthernetInterface InterfaceType = "ethernet"

	// VLANInterface is used for interfaces representing IEEE 802.11Q VLAN
	// devices.
	VLANInterface InterfaceType = "vlan"

	// BondInterface is used for interfaces representing bonding devices.
	BondInterface InterfaceType = "bond"

	// BridgeInterface is used for interfaces represending an OSI layer-2 bridge
	// devices.
	BridgeInterface InterfaceType = "bridge"
)

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

// ProviderID returns the provider-specific interface ID, if set. The result
// does not include the model UUID.
func (nic *Interface) ProviderID() network.Id {
	localProviderID := nic.st.localID(nic.doc.ProviderID)
	return network.Id(localProviderID)
}

// MachineID returns the ID of the machine this interface is on.
func (nic *Interface) MachineID() string {
	return nic.doc.MachineID
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

// IsActive returns whether the interface is currently active and usable.
func (nic *Interface) IsActive() bool {
	return nic.doc.IsActive
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

	return nic.st.Interface(nic.doc.ParentName)
}

// Interface returns the interface matching the given name. An error satisfying
// errors.IsNotFound() is returned when no such interface exists.
func (st *State) Interface(name string) (*Interface, error) {
	interfaces, closer := st.getCollection(interfacesC)
	defer closer()

	var doc interfaceDoc
	err := interfaces.FindId(name).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("interface %q", name)
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot get interface %q", name)
	}
	return newInterface(st, doc), nil
}

// DNSServers returns the list of DNS nameservers that apply to this interface,
// if they are known, or an empty slice otherwise.
func (nic *Interface) DNSServers() []string {
	return nic.doc.DNSServers
}

// DNSSDomain returns the default DNS domain that applies to this interface, if
// set.
func (nic *Interface) DNSDomain() string {
	return nic.doc.DNSDomain
}

// GatewayAddress returns the address of the gateway to use for this interface,
// if set.
func (nic *Interface) GatewayAddress() string {
	return nic.doc.GatewayAddress
}

func (nic *Interface) globalKey() string {
	return interfaceGlobalKey(nic.doc.MachineID, nic.doc.Name)
}

// String returns the interface as a human-readable string.
func (nic *Interface) String() string {
	return fmt.Sprintf("interface %q on machine %q", nic.doc.Name, nic.doc.MachineID)
}

// interfaceGlobalKey returns a (model-unique) global key for an interface. If
// either argument is empty, the result is empty.
func interfaceGlobalKey(machineID, interfaceName string) string {
	if machineID == "" || interfaceName == "" {
		return ""
	}
	return "m#" + machineID + "i#" + interfaceName
}

// isValidInterfaceName returns whether the given interfaceName is a valid
// network device name, depending on the runtime.GOOS value.
func isValidInterfaceName(interfaceName string) bool {
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
	if minLength > maxLength {
		minLength, maxLength = maxLength, minLength
	}
	return length >= minLength && length <= maxLength
}

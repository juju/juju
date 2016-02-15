// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

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
	ProviderID string `bson:"provider-id,omitempty"`

	// MachineID is the ID of the machine where this interface is located.
	MachineID string `bson:"machine-id"`

	// Type is the type of the interface related to the underlying device.
	Type interfaceType `bson:"type"`

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

// interfaceType defines the type of a machine network interface.
type interfaceType string

const (
	// unknownInterface is used for interfaces with unknown type.
	unknownInterface interfaceType = "unknown"

	// loopbackInterface is used for loopback interfaces.
	loopbackInterface interfaceType = "loopback"

	// ethernetInterface is used for interfaces representing Ethernet devices.
	ethernetInterface interfaceType = "ethernet"

	// vlanInterface is used for interfaces representing IEEE 802.11Q VLAN
	// devices.
	vlanInterface interfaceType = "vlan"

	// bondInterface is used for interfaces representing bonding devices.
	bondInterface interfaceType = "bond"

	// bridgeInterface is used for interfaces represending an OSI layer-2 bridge
	// devices.
	bridgeInterface interfaceType = "bridge"
)

// Interface represents the state of a machine network interface.
type Interface struct {
	st  *State
	doc interfaceDoc
}

func newInterface(st *State, doc *interfaceDoc) *Interface {
	return &Interface{
		st:  st,
		doc: *doc,
	}
}

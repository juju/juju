// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// interfaceLinkDoc describes the persistent state of a network interface link.
type interfaceLinkDoc struct {
	// DocID is the interface link ID, prefixed by ModelUUID.
	DocID string `bson:"_id"`

	// LinkID is the ID of the link, which is unique within the model.
	LinkID string `bson:"link-id"`

	// ModelUUID is the UUID of the model this link is part of.
	ModelUUID string `bson:"model-uuid"`

	// ProviderID is a provider-specific ID of the link, prefixed by
	// ModelUUID. Empty when not supported by the provider.
	ProviderID string `bson:"provider-id,omitempty"`

	// InterfaceName is the name of the interface this link belongs to.
	InterfaceName string `bson:"interface-name"`

	// MachineID is the ID of the machine this link's interface belongs to.
	MachineID string `bson:"machine-id"`

	// SubnetID is the ID of the subnet this link got its address from.
	SubnetID string `bson:"subnet-id"`

	// Method is the method used to configure this link.
	Method linkMethod `bson:"method"`

	// IsActive is true when the link is up.
	IsActive bool `bson:"is-active"`

	// Address is the address this link uses.
	Address string `bson:"address"`
}

// linkMethod is the method used for a network interface link.
type linkMethod string

const (
	// unknownLink is used for links with unknown method.
	unknownLink linkMethod = "unknown"

	// staticLink is used for statically configured links.
	staticLink linkMethod = "static"

	// dhcpLink is used for DHCP-configured links.
	dhcpLink linkMethod = "dhcp"

	// manualLink is used for manually configured links,
	manualLink linkMethod = "manual"
)

// InterfaceLink represents the state of a network interface link.
type InterfaceLink struct {
	st  *State
	doc interfaceLinkDoc
}

func newInterfaceLink(st *State, doc *interfaceLinkDoc) *InterfaceLink {
	return &InterfaceLink{
		st:  st,
		doc: *doc,
	}
}

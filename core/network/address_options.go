// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

// AddressMutator describes setter methods for an address.
type AddressMutator interface {
	// SetScope sets the scope property of the address.
	SetScope(Scope)

	// SetCIDR sets the CIDR property of the address.
	SetCIDR(string)

	// SetSecondary indicates whether this address is not the
	// primary address of the device it is associated with.
	SetSecondary(bool)

	// SetConfigType indicates how this address was configured.
	SetConfigType(AddressConfigType)
}

// SetScope (AddressMutator) sets the input
// scope on the address receiver.
func (a *MachineAddress) SetScope(scope Scope) {
	a.Scope = scope
}

// SetCIDR (AddressMutator) sets the input
// CIDR on the address receiver.
func (a *MachineAddress) SetCIDR(cidr string) {
	a.CIDR = cidr
}

// SetSecondary (AddressMutator) sets the IsSecondary
// member to true on the address receiver.
func (a *MachineAddress) SetSecondary(isSecondary bool) {
	a.IsSecondary = isSecondary
}

// SetConfigType (AddressMutator) sets the input
// AddressConfigType on the address receiver.
func (a *MachineAddress) SetConfigType(configType AddressConfigType) {
	a.ConfigType = configType
}

// WithScope returns a functional option that can
// be used to set the input scope on an address.
func WithScope(scope Scope) func(AddressMutator) {
	return func(a AddressMutator) {
		a.SetScope(scope)
	}
}

// WithCIDR returns a functional option that can
// be used to set the input CIDR on an address.
func WithCIDR(cidr string) func(AddressMutator) {
	return func(a AddressMutator) {
		a.SetCIDR(cidr)
	}
}

// WithSecondary returns a functional option that can be used to
// indicate whether an address is not the primary for its NIC.
func WithSecondary(isSecondary bool) func(AddressMutator) {
	return func(a AddressMutator) {
		a.SetSecondary(isSecondary)
	}
}

func WithConfigType(configType AddressConfigType) func(AddressMutator) {
	return func(a AddressMutator) {
		a.SetConfigType(configType)
	}
}

// ProviderAddressMutator describes setter methods for a ProviderAddress
type ProviderAddressMutator interface {
	AddressMutator

	// SetSpaceName sets the SpaceName property of the provider address
	SetSpaceName(string)

	// SetProviderSpaceID sets the ProviderSpaceID property of the provider address
	SetProviderSpaceID(Id)

	// SetProviderID sets the ProviderID property of the provider address
	SetProviderID(Id)

	// SetProviderSubnetID sets the ProviderSubnetID property of the provider address
	SetProviderSubnetID(Id)

	// SetProviderVLANID sets the ProviderVLANID property of the provider address
	SetProviderVLANID(Id)

	// SetVLANTag sets the VLANTag property of the provider address
	SetVLANTag(int)
}

// SetSpaceName (ProviderAddressMutator) sets the input
// space name on the provider address receiver
func (a *ProviderAddress) SetSpaceName(spaceName string) {
	a.SpaceName = NewSpaceName(spaceName)
}

// SetProviderSpaceID (ProviderAddressMutator) sets the input
// provider space id on the provider address receiver
func (a *ProviderAddress) SetProviderSpaceID(id Id) {
	a.ProviderSpaceID = id
}

// SetProviderID (ProviderAddressMutator) sets the input
// provider id on the provider address receiver
func (a *ProviderAddress) SetProviderID(id Id) {
	a.ProviderID = id
}

// SetProviderSubnetID (ProviderAddressMutator) sets the input
// provider subnet id on the provider addrerss reviever
func (a *ProviderAddress) SetProviderSubnetID(id Id) {
	a.ProviderSubnetID = id
}

// SetProviderVLANID (ProviderAddressMutator) sets the input
// provider VLAN id on the provider addrerss reviever
func (a *ProviderAddress) SetProviderVLANID(id Id) {
	a.ProviderVLANID = id
}

// SetVLANTag (ProviderAddressMutator) sets the input
// VLAN tag on the provider addrerss reviever
func (a *ProviderAddress) SetVLANTag(tag int) {
	a.VLANTag = tag
}

// WithSpaceName returns a functional option that can
// be used to set the input space name on a provider address.
func WithSpaceName(space string) func(ProviderAddressMutator) {
	return func(a ProviderAddressMutator) {
		a.SetSpaceName(space)
	}
}

// WithProviderSpaceID returns a functional option that can
// be used to set the input provider space id on a provider address
func WithProviderSpaceID(id Id) func(ProviderAddressMutator) {
	return func(a ProviderAddressMutator) {
		a.SetProviderSpaceID(id)
	}
}

// WithProviderID returns a functional option that can
// be used to set the input provider id on a provider address
func WithProviderID(id Id) func(ProviderAddressMutator) {
	return func(a ProviderAddressMutator) {
		a.SetProviderID(id)
	}
}

// WithProviderSubnetID returns a functional option that can
// be used to set the input provider subnet id on a provider address
func WithProviderSubnetID(id Id) func(ProviderAddressMutator) {
	return func(a ProviderAddressMutator) {
		a.SetProviderSubnetID(id)
	}
}

// WithProviderVLANID returns a functional option that can
// be used to set the input provider VLAN id on a provider address
func WithProviderVLANID(id Id) func(ProviderAddressMutator) {
	return func(a ProviderAddressMutator) {
		a.SetProviderVLANID(id)
	}
}

// WithVLANTag returns a functional option that can
// be used to set the input VLAN tag on a provider address
func WithVLANTag(tag int) func(ProviderAddressMutator) {
	return func(a ProviderAddressMutator) {
		a.SetVLANTag(tag)
	}
}

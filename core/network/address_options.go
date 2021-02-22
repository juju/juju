// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

// AddressMutator describes setter methods for an address.
type AddressMutator interface {
	// SetScope sets the scope property of the address.
	SetScope(Scope)

	// SetCIDR sets the CIDR property of the address.
	SetCIDR(string)
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

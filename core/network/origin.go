// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

// Origin specifies where an address comes from, whether it was reported by a
// provider or by a machine.
type Origin string

const (
	// OriginUnknown address origin unknown.
	OriginUnknown Origin = ""
	// OriginProvider address comes from a provider.
	OriginProvider Origin = "provider"
	// OriginMachine address comes from a machine.
	OriginMachine Origin = "machine"
)

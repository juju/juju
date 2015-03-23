// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// -----
// Parameters field types.
// -----

// Network describes a single network available on an instance.
type Space struct {
	// Tag is the network's tag.
	Tag string `json:"Tag"`

	// ProviderId is the provider-specific network id.
	ProviderId string `json:"ProviderId"`

	// CIDR of the network, in "123.45.67.89/12" format.
	CIDR string `json:"CIDR"`

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard.
	VLANTag int `json:"VLANTag"`
}

// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

// FanCIDRs describes the subnets relevant to a fan network.
type FanCIDRs struct {
	// FanLocalUnderlay is the CIDR of the local underlying fan network.
	// It allows easy identification of the device the fan is running on.
	FanLocalUnderlay string

	// FanOverlay is the CIDR of the complete fan setup.
	FanOverlay string
}

func newFanCIDRs(overlay, underlay string) *FanCIDRs {
	return &FanCIDRs{
		FanLocalUnderlay: underlay,
		FanOverlay:       overlay,
	}
}

// SubnetInfo is a source-agnostic representation of a subnet.
// It may originate from state, or from a provider.
type SubnetInfo struct {
	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// ProviderId is a provider-specific subnet ID.
	ProviderId Id

	// ProviderSpaceId holds the provider ID of the space associated
	// with this subnet. Can be empty if not supported.
	ProviderSpaceId Id

	// ProviderNetworkId holds the provider ID of the network
	// containing this subnet, for example VPC id for EC2.
	ProviderNetworkId Id

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard, and used
	// to define a VLAN network. For more information, see:
	// http://en.wikipedia.org/wiki/IEEE_802.1Q.
	VLANTag int

	// AvailabilityZones describes which availability zones this
	// subnet is in. It can be empty if the provider does not support
	// availability zones.
	AvailabilityZones []string

	// SpaceName is the name of the space the subnet is associated with.
	// It can be empty if the subnet is not associated with a space yet.
	// TODO (manadart 2019-07-11): Remove in lieu of SpaceId?
	SpaceName string

	// FanInfo describes the fan networking setup for the subnet.
	// It may be empty if this is not a fan subnet,
	// or if this subnet information comes from a provider.
	FanInfo *FanCIDRs
}

// SetFan sets the fan networking information for the subnet.
func (s *SubnetInfo) SetFan(underlay, overlay string) {
	s.FanInfo = newFanCIDRs(overlay, underlay)
}

// FanLocalUnderlay returns the fan underlay CIDR if known.
func (s *SubnetInfo) FanLocalUnderlay() string {
	if s.FanInfo == nil {
		return ""
	}
	return s.FanInfo.FanLocalUnderlay
}

// FanOverlay returns the fan overlay CIDR if known.
func (s *SubnetInfo) FanOverlay() string {
	if s.FanInfo == nil {
		return ""
	}
	return s.FanInfo.FanOverlay
}

// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

const (
	// DefaultSpaceId is the ID of the default network space.
	// Application endpoints are bound to this space by default
	// if no explicit binding is specified.
	DefaultSpaceId = "0"

	// DefaultSpaceName is the name of the default network space.
	DefaultSpaceName = ""
)

// SpaceInfo defines a network space.
type SpaceInfo struct {
	// Name is the name of the space.
	// It is used by operators for identifying a space and should be unique.
	Name string

	// ProviderId is the provider's unique identifier for the space,
	// such as used by MAAS.
	ProviderId Id

	// Subnets are the subnets that have been grouped into this network space.
	Subnets []SubnetInfo
}

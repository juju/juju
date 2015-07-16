// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	providercommon "github.com/juju/juju/provider/common"
)

// BackingSubnet defines the methods supported by a Subnet entity
// stored persistently.
//
// TODO(dimitern): Once the state backing is implemented, remove this
// and just use *state.Subnet.
type BackingSubnet interface {
	// no methods needed yet.
}

// BackingSpace defines the methods supported by a Space entity stored
// persistently.
type BackingSpace interface {
	// Name returns the space name.
	Name() string

	// Subnets returns the subnets in the space
	Subnets() ([]BackingSubnet, error)

	// ProviderId returns the network ID of the provider
	ProviderId() network.Id

	// Zones returns a list of availability zone(s) that this
	// space is in. It can be empty if the provider does not support
	// availability zones.
	Zones() []string

	// Life returns the lifecycle state of the space
	Life() params.Life
}

// Backing defines the methods needed by the API facade to store and
// retrieve information from the underlying persistency layer (state
// DB).
type NetworkBacking interface {
	// EnvironConfig returns the current environment config.
	EnvironConfig() (*config.Config, error)

	// AvailabilityZones returns all cached availability zones (i.e.
	// not from the provider, but in state).
	AvailabilityZones() ([]providercommon.AvailabilityZone, error)

	// SetAvailabilityZones replaces the cached list of availability
	// zones with the given zones.
	SetAvailabilityZones(zones []providercommon.AvailabilityZone) error
}
